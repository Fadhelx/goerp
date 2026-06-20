package mail

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPProviderSenderPostsPayload(t *testing.T) {
	var payload HTTPProviderPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content-type = %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	sender, err := NewHTTPProviderSender(HTTPProviderConfig{
		Endpoint: server.URL,
		From:     "sender@example.com",
		Headers:  map[string]string{"Authorization": "Bearer test-token"},
		Client:   server.Client(),
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(Message{
		To:      "recipient@example.com",
		Subject: "Provider",
		Body:    "<p>Body</p>",
	}); err != nil {
		t.Fatal(err)
	}

	expected := HTTPProviderPayload{
		From:    "sender@example.com",
		To:      "recipient@example.com",
		Subject: "Provider",
		Body:    "<p>Body</p>",
	}
	if payload != expected {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestHTTPProviderSenderDoesNotExposeResponseBodyInOutboxError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider password token leaked", http.StatusBadGateway)
	}))
	defer server.Close()

	sender, err := NewHTTPProviderSender(HTTPProviderConfig{
		Endpoint: server.URL,
		From:     "sender@example.com",
		Client:   server.Client(),
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	directErr := sender.Send(Message{To: "recipient@example.com"})
	if directErr == nil {
		t.Fatal("expected direct sender error")
	}
	if strings.Contains(directErr.Error(), "password") || strings.Contains(directErr.Error(), "token") {
		t.Fatalf("unsafe direct error = %q", directErr.Error())
	}

	now := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	outbox := NewOutbox()
	id, err := outbox.Enqueue(Message{To: "recipient@example.com", Subject: "S", Body: "B"}, now)
	if err != nil {
		t.Fatal(err)
	}
	result := outbox.SendDue(sender, now)
	if result.Retried != 1 || result.Sent != 0 || result.Dead != 0 {
		t.Fatalf("result = %+v", result)
	}
	message, ok := outbox.Get(id)
	if !ok {
		t.Fatal("message missing")
	}
	if message.LastError != "send failed" {
		t.Fatalf("last error = %q", message.LastError)
	}
}
