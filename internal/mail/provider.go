package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPProviderConfig struct {
	Endpoint string
	From     string
	Headers  map[string]string
	Client   *http.Client
	Timeout  time.Duration
}

type HTTPProviderPayload struct {
	From    string `json:"from"`
	To      string `json:"to"`
	CC      string `json:"cc,omitempty"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type HTTPProviderSender struct {
	config HTTPProviderConfig
}

func NewHTTPProviderSender(config HTTPProviderConfig) (*HTTPProviderSender, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &HTTPProviderSender{config: config}, nil
}

func (s *HTTPProviderSender) Send(message Message) error {
	if s == nil {
		return fmt.Errorf("http provider sender unavailable")
	}
	if err := s.config.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(message.To) == "" && strings.TrimSpace(message.CC) == "" {
		return fmt.Errorf("mail recipients required")
	}
	payload := HTTPProviderPayload{
		From:    s.config.From,
		To:      message.To,
		CC:      message.CC,
		Subject: message.Subject,
		Body:    message.Body,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("http provider payload failed: %w", err)
	}

	timeout := s.config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("http provider request failed: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	for key, value := range s.config.Headers {
		req.Header.Set(key, value)
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return fmt.Errorf("http provider send failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("http provider status %d", resp.StatusCode)
	}
	return nil
}

func (s *HTTPProviderSender) client() *http.Client {
	if s.config.Client != nil {
		return s.config.Client
	}
	return http.DefaultClient
}

func (c HTTPProviderConfig) validate() error {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		return fmt.Errorf("http provider endpoint required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("http provider endpoint invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("http provider endpoint scheme invalid")
	}
	if strings.TrimSpace(c.From) == "" {
		return fmt.Errorf("http provider from required")
	}
	for key := range c.Headers {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("http provider header invalid")
		}
	}
	return nil
}
