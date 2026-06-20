package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
	"time"
)

type SMTPTLSMode string

const (
	SMTPTLSNone     SMTPTLSMode = "none"
	SMTPTLSStartTLS SMTPTLSMode = "starttls"
	SMTPTLSImplicit SMTPTLSMode = "tls"
)

type SMTPConfig struct {
	Host      string
	Port      int
	Username  string
	Password  string
	From      string
	TLSMode   SMTPTLSMode
	TLSConfig *tls.Config
	Timeout   time.Duration
}

type SMTPSender struct {
	config      SMTPConfig
	dialContext func(context.Context, string, string) (net.Conn, error)
}

func NewSMTPSender(config SMTPConfig) (*SMTPSender, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &SMTPSender{config: config}, nil
}

func (s *SMTPSender) Send(message Message) error {
	if s == nil {
		return fmt.Errorf("smtp sender unavailable")
	}
	if err := s.config.validate(); err != nil {
		return err
	}
	fromHeader := strings.TrimSpace(message.From)
	if fromHeader == "" {
		fromHeader = s.config.From
	}
	if _, err := parseSingleAddress(fromHeader); err != nil {
		return err
	}
	envelopeHeader := strings.TrimSpace(message.EnvelopeFrom)
	if envelopeHeader == "" {
		envelopeHeader = messageHeaderValue(message.Headers, "Return-Path")
	}
	if envelopeHeader == "" {
		envelopeHeader = fromHeader
	}
	envelopeFrom, err := parseSingleAddress(envelopeHeader)
	if err != nil {
		return err
	}
	recipients := []string{}
	if strings.TrimSpace(message.To) != "" {
		toRecipients, err := parseAddresses(message.To)
		if err != nil {
			return err
		}
		recipients = append(recipients, toRecipients...)
	}
	if strings.TrimSpace(message.CC) != "" {
		ccRecipients, err := parseAddresses(message.CC)
		if err != nil {
			return err
		}
		recipients = append(recipients, ccRecipients...)
	}
	if len(recipients) == 0 {
		return fmt.Errorf("mail recipients required")
	}
	data, err := buildSMTPMessage(fromHeader, message)
	if err != nil {
		return err
	}

	timeout := s.config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	address := net.JoinHostPort(s.config.Host, strconv.Itoa(s.config.Port))
	client, err := s.smtpClient(ctx, address)
	if err != nil {
		return err
	}
	defer client.Close()

	if normalizedTLSMode(s.config.TLSMode) == SMTPTLSStartTLS {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			return fmt.Errorf("smtp starttls unsupported")
		}
		if err := client.StartTLS(s.tlsConfig()); err != nil {
			return fmt.Errorf("smtp starttls failed: %w", err)
		}
	}

	if s.config.Username != "" {
		auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth failed: %w", err)
		}
	}
	if err := client.Mail(envelopeFrom); err != nil {
		return fmt.Errorf("smtp mail failed: %w", err)
	}
	acceptedRecipients := 0
	var firstRecipientError error
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			if smtpRecipientErrorCloses(err) {
				return fmt.Errorf("smtp rcpt failed: %w", err)
			}
			if firstRecipientError == nil {
				firstRecipientError = err
			}
			continue
		}
		acceptedRecipients++
	}
	if acceptedRecipients == 0 {
		if firstRecipientError != nil {
			return fmt.Errorf("smtp rcpt failed: %w", firstRecipientError)
		}
		return fmt.Errorf("smtp rcpt failed")
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data failed: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		return fmt.Errorf("smtp write failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("smtp data close failed: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp quit failed: %w", err)
	}
	return nil
}

func smtpRecipientErrorCloses(err error) bool {
	var textErr *textproto.Error
	return errors.As(err, &textErr) && textErr.Code == 421
}

func messageHeaderValue(headers map[string]string, key string) string {
	for headerKey, value := range headers {
		if strings.EqualFold(strings.TrimSpace(headerKey), key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *SMTPSender) smtpClient(ctx context.Context, address string) (*smtp.Client, error) {
	conn, err := s.dial(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("smtp connect failed: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if normalizedTLSMode(s.config.TLSMode) == SMTPTLSImplicit {
		tlsConn := tls.Client(conn, s.tlsConfig())
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("smtp tls failed: %w", err)
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, s.config.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("smtp client failed: %w", err)
	}
	return client, nil
}

func (s *SMTPSender) dial(ctx context.Context, address string) (net.Conn, error) {
	if s.dialContext != nil {
		return s.dialContext(ctx, "tcp", address)
	}
	dialer := net.Dialer{}
	return dialer.DialContext(ctx, "tcp", address)
}

func (s *SMTPSender) tlsConfig() *tls.Config {
	if s.config.TLSConfig == nil {
		return &tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}
	}
	cfg := s.config.TLSConfig.Clone()
	if cfg.ServerName == "" {
		cfg.ServerName = s.config.Host
	}
	if cfg.MinVersion == 0 {
		cfg.MinVersion = tls.VersionTLS12
	}
	return cfg
}

func (c SMTPConfig) validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("smtp host required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("smtp port invalid")
	}
	if _, err := parseSingleAddress(c.From); err != nil {
		return err
	}
	switch normalizedTLSMode(c.TLSMode) {
	case SMTPTLSNone, SMTPTLSStartTLS, SMTPTLSImplicit:
		return nil
	default:
		return fmt.Errorf("smtp tls mode invalid")
	}
}

func normalizedTLSMode(mode SMTPTLSMode) SMTPTLSMode {
	if mode == "" {
		return SMTPTLSStartTLS
	}
	return mode
}

func parseSingleAddress(value string) (string, error) {
	if err := rejectHeaderInjection(value); err != nil {
		return "", err
	}
	address, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("mail from invalid: %w", err)
	}
	return address.Address, nil
}

func parseAddresses(value string) ([]string, error) {
	if err := rejectHeaderInjection(value); err != nil {
		return nil, err
	}
	addresses, err := mail.ParseAddressList(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("mail recipients invalid: %w", err)
	}
	recipients := make([]string, 0, len(addresses))
	for _, address := range addresses {
		recipients = append(recipients, address.Address)
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("mail recipients required")
	}
	return recipients, nil
}

func buildSMTPMessage(from string, message Message) ([]byte, error) {
	if err := rejectHeaderInjection(from); err != nil {
		return nil, err
	}
	if err := rejectHeaderInjection(message.To); err != nil {
		return nil, err
	}
	if err := rejectHeaderInjection(message.CC); err != nil {
		return nil, err
	}
	if err := rejectHeaderInjection(message.ReplyTo); err != nil {
		return nil, err
	}
	if err := rejectHeaderInjection(message.Subject); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	writeHeader(&buf, "From", from)
	writeHeader(&buf, "To", message.To)
	if strings.TrimSpace(message.CC) != "" {
		writeHeader(&buf, "Cc", message.CC)
	}
	if strings.TrimSpace(message.ReplyTo) != "" {
		writeHeader(&buf, "Reply-To", message.ReplyTo)
	}
	writeHeader(&buf, "Subject", mime.QEncoding.Encode("utf-8", message.Subject))
	writeHeader(&buf, "MIME-Version", "1.0")
	if err := writeExtraHeaders(&buf, message.Headers); err != nil {
		return nil, err
	}
	if len(message.Attachments) > 0 {
		return buildMultipartSMTPMessage(&buf, message)
	}
	writeHeader(&buf, "Content-Type", `text/html; charset="utf-8"`)
	writeHeader(&buf, "Content-Transfer-Encoding", "8bit")
	buf.WriteString("\r\n")
	buf.WriteString(normalizeCRLF(message.Body))
	if !strings.HasSuffix(buf.String(), "\r\n") {
		buf.WriteString("\r\n")
	}
	return buf.Bytes(), nil
}

func buildMultipartSMTPMessage(buf *bytes.Buffer, message Message) ([]byte, error) {
	writer := multipart.NewWriter(buf)
	writeHeader(buf, "Content-Type", mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": writer.Boundary()}))
	buf.WriteString("\r\n")
	bodyHeader := textproto.MIMEHeader{}
	bodyHeader.Set("Content-Type", `text/html; charset="utf-8"`)
	bodyHeader.Set("Content-Transfer-Encoding", "8bit")
	bodyPart, err := writer.CreatePart(bodyHeader)
	if err != nil {
		return nil, err
	}
	if _, err := bodyPart.Write([]byte(normalizeCRLF(message.Body))); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(message.Body, "\n") {
		if _, err := bodyPart.Write([]byte("\r\n")); err != nil {
			return nil, err
		}
	}
	for _, attachment := range message.Attachments {
		name := strings.TrimSpace(attachment.Name)
		if name == "" {
			name = "attachment"
		}
		if err := rejectHeaderInjection(name); err != nil {
			return nil, err
		}
		contentType := strings.TrimSpace(attachment.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		if err := rejectHeaderInjection(contentType); err != nil {
			return nil, err
		}
		partHeader := textproto.MIMEHeader{}
		partHeader.Set("Content-Type", mime.FormatMediaType(contentType, map[string]string{"name": name}))
		partHeader.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		partHeader.Set("Content-Transfer-Encoding", "base64")
		part, err := writer.CreatePart(partHeader)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write([]byte(base64Lines(attachment.Data))); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeExtraHeaders(buf *bytes.Buffer, headers map[string]string) error {
	names := make([]string, 0, len(headers))
	for key := range headers {
		names = append(names, key)
	}
	sort.Strings(names)
	for _, key := range names {
		value := headers[key]
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, ":\r\n") {
			return fmt.Errorf("mail header invalid")
		}
		if err := rejectHeaderInjection(value); err != nil {
			return err
		}
		writeHeader(buf, key, value)
	}
	return nil
}

func base64Lines(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	if encoded == "" {
		return "\r\n"
	}
	var b strings.Builder
	for len(encoded) > 76 {
		b.WriteString(encoded[:76])
		b.WriteString("\r\n")
		encoded = encoded[76:]
	}
	b.WriteString(encoded)
	b.WriteString("\r\n")
	return b.String()
}

func writeHeader(buf *bytes.Buffer, key, value string) {
	buf.WriteString(key)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteString("\r\n")
}

func rejectHeaderInjection(value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("mail header contains newline")
	}
	return nil
}

func normalizeCRLF(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.ReplaceAll(value, "\n", "\r\n")
}
