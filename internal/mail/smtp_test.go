package mail

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSMTPSenderSendsMessageToLocalServer(t *testing.T) {
	server := newFakeSMTPServer(t, nil)
	defer server.Close()

	sender, err := NewSMTPSender(SMTPConfig{
		Host:    server.host,
		Port:    server.port,
		From:    "Sender <sender@example.com>",
		TLSMode: SMTPTLSNone,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(Message{
		To:      "Recipient <recipient@example.com>",
		Subject: "Hello",
		Body:    "<p>Body</p>",
	}); err != nil {
		t.Fatal(err)
	}

	server.wait(t)
	if server.mailFrom != "sender@example.com" {
		t.Fatalf("mail from = %q", server.mailFrom)
	}
	if len(server.rcptTo) != 1 || server.rcptTo[0] != "recipient@example.com" {
		t.Fatalf("rcpt to = %+v", server.rcptTo)
	}
	if !strings.Contains(server.data, "Subject: Hello\r\n") {
		t.Fatalf("missing subject: %q", server.data)
	}
	if !strings.Contains(server.data, "From: Sender <sender@example.com>\r\n") {
		t.Fatalf("missing from: %q", server.data)
	}
	if !strings.Contains(server.data, "To: Recipient <recipient@example.com>\r\n") {
		t.Fatalf("missing to: %q", server.data)
	}
	if !strings.Contains(server.data, "\r\n\r\n<p>Body</p>\r\n") {
		t.Fatalf("missing body: %q", server.data)
	}
}

func TestSMTPSenderUsesEnvelopeFromWithoutChangingHeaderFrom(t *testing.T) {
	server := newFakeSMTPServer(t, nil)
	defer server.Close()

	sender, err := NewSMTPSender(SMTPConfig{
		Host:    server.host,
		Port:    server.port,
		From:    "fallback@example.com",
		TLSMode: SMTPTLSNone,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(Message{
		From:         "Sender <sender@example.com>",
		EnvelopeFrom: "bounce@example.com",
		To:           "recipient@example.com",
		Subject:      "Envelope",
		Body:         "<p>Body</p>",
	}); err != nil {
		t.Fatal(err)
	}

	server.wait(t)
	if server.mailFrom != "bounce@example.com" {
		t.Fatalf("mail from = %q", server.mailFrom)
	}
	if !strings.Contains(server.data, "From: Sender <sender@example.com>\r\n") {
		t.Fatalf("missing header from: %q", server.data)
	}
}

func TestSMTPSenderBuildsMultipartAttachments(t *testing.T) {
	server := newFakeSMTPServer(t, nil)
	defer server.Close()

	sender, err := NewSMTPSender(SMTPConfig{
		Host:    server.host,
		Port:    server.port,
		From:    "fallback@example.com",
		TLSMode: SMTPTLSNone,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(Message{
		From:    "Sender <sender@example.com>",
		To:      "recipient@example.com",
		CC:      "copy@example.com",
		ReplyTo: "reply@example.com",
		Subject: "With file",
		Body:    "<p>Body</p>",
		Headers: map[string]string{"X-Test": "ok"},
		Attachments: []Attachment{{
			Name:        "report.txt",
			ContentType: "text/plain",
			Data:        []byte("sample"),
		}},
	}); err != nil {
		t.Fatal(err)
	}

	server.wait(t)
	if len(server.rcptTo) != 2 || server.rcptTo[0] != "recipient@example.com" || server.rcptTo[1] != "copy@example.com" {
		t.Fatalf("rcpt to = %+v", server.rcptTo)
	}
	for _, needle := range []string{
		"From: Sender <sender@example.com>\r\n",
		"Reply-To: reply@example.com\r\n",
		"X-Test: ok\r\n",
		"Content-Type: multipart/mixed;",
		"Content-Type: text/html; charset=\"utf-8\"",
		"Content-Disposition: attachment; filename=report.txt",
		"Content-Type: text/plain; name=report.txt",
		"c2FtcGxl",
	} {
		if !strings.Contains(server.data, needle) {
			t.Fatalf("missing %q in %q", needle, server.data)
		}
	}
}

func TestSMTPSenderContinuesAfterPartialRecipientRefusal(t *testing.T) {
	server := newFakeSMTPServer(t, func(line string) (string, bool) {
		if strings.HasPrefix(line, "RCPT TO:") && strings.Contains(line, "bad@example.com") {
			return "550 bad recipient\r\n", true
		}
		return "", false
	})
	defer server.Close()

	sender, err := NewSMTPSender(SMTPConfig{
		Host:    server.host,
		Port:    server.port,
		From:    "sender@example.com",
		TLSMode: SMTPTLSNone,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(Message{
		To:      "bad@example.com, good@example.com",
		Subject: "Partial",
		Body:    "<p>Body</p>",
	}); err != nil {
		t.Fatal(err)
	}

	server.wait(t)
	if len(server.rcptTo) != 2 || server.rcptTo[0] != "bad@example.com" || server.rcptTo[1] != "good@example.com" || len(server.acceptedRcptTo) != 1 || server.acceptedRcptTo[0] != "good@example.com" || len(server.refusedRcptTo) != 1 || server.refusedRcptTo[0] != "bad@example.com" {
		t.Fatalf("server state rcpt=%+v accepted=%+v refused=%+v", server.rcptTo, server.acceptedRcptTo, server.refusedRcptTo)
	}
	if !strings.Contains(server.data, "Subject: Partial\r\n") {
		t.Fatalf("missing data: %q", server.data)
	}
}

func TestSMTPSenderFailsWhenAllRecipientsRefused(t *testing.T) {
	server := newFakeSMTPServer(t, func(line string) (string, bool) {
		if strings.HasPrefix(line, "RCPT TO:") {
			return "550 bad recipient\r\n", true
		}
		return "", false
	})
	defer server.Close()

	sender, err := NewSMTPSender(SMTPConfig{
		Host:    server.host,
		Port:    server.port,
		From:    "sender@example.com",
		TLSMode: SMTPTLSNone,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = sender.Send(Message{
		To:      "bad@example.com",
		CC:      "worse@example.com",
		Subject: "Refused",
		Body:    "<p>Body</p>",
	})
	if err == nil || !strings.Contains(err.Error(), "smtp rcpt failed") {
		t.Fatalf("send error = %v", err)
	}

	server.wait(t)
	if len(server.rcptTo) != 2 || len(server.acceptedRcptTo) != 0 || len(server.refusedRcptTo) != 2 || server.data != "" {
		t.Fatalf("server state rcpt=%+v accepted=%+v refused=%+v data=%q", server.rcptTo, server.acceptedRcptTo, server.refusedRcptTo, server.data)
	}
}

func TestSMTPSenderFailsImmediatelyOnClosingRecipientRefusal(t *testing.T) {
	server := newFakeSMTPServer(t, func(line string) (string, bool) {
		if strings.HasPrefix(line, "RCPT TO:") && strings.Contains(line, "bad@example.com") {
			return "421 service unavailable\r\n", true
		}
		return "", false
	})
	defer server.Close()

	sender, err := NewSMTPSender(SMTPConfig{
		Host:    server.host,
		Port:    server.port,
		From:    "sender@example.com",
		TLSMode: SMTPTLSNone,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = sender.Send(Message{
		To:      "good@example.com, bad@example.com",
		Subject: "Closing",
		Body:    "<p>Body</p>",
	})
	if err == nil || !strings.Contains(err.Error(), "smtp rcpt failed") {
		t.Fatalf("send error = %v", err)
	}

	server.wait(t)
	if len(server.rcptTo) != 2 || server.rcptTo[0] != "good@example.com" || server.rcptTo[1] != "bad@example.com" || len(server.acceptedRcptTo) != 1 || server.acceptedRcptTo[0] != "good@example.com" || len(server.refusedRcptTo) != 1 || server.refusedRcptTo[0] != "bad@example.com" || server.data != "" {
		t.Fatalf("server state rcpt=%+v accepted=%+v refused=%+v data=%q", server.rcptTo, server.acceptedRcptTo, server.refusedRcptTo, server.data)
	}
}

func TestSMTPSenderFailureStoredAsSanitizedOutboxError(t *testing.T) {
	server := newFakeSMTPServer(t, func(line string) (string, bool) {
		if strings.HasPrefix(line, "RCPT TO:") {
			return "550 smtp password token leaked\r\n", true
		}
		return "", false
	})
	defer server.Close()

	sender, err := NewSMTPSender(SMTPConfig{
		Host:    server.host,
		Port:    server.port,
		From:    "sender@example.com",
		TLSMode: SMTPTLSNone,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
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

type fakeSMTPServer struct {
	host     string
	port     int
	listener net.Listener
	done     chan struct{}

	mu             sync.Mutex
	err            error
	mailFrom       string
	rcptTo         []string
	acceptedRcptTo []string
	refusedRcptTo  []string
	data           string
	override       func(string) (string, bool)
}

func newFakeSMTPServer(t *testing.T, override func(string) (string, bool)) *fakeSMTPServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, portValue, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatal(err)
	}
	server := &fakeSMTPServer{
		host:     host,
		port:     port,
		listener: listener,
		done:     make(chan struct{}),
		override: override,
	}
	go server.serve()
	return server
}

func (s *fakeSMTPServer) Close() {
	_ = s.listener.Close()
	select {
	case <-s.done:
	case <-time.After(time.Second):
	}
}

func (s *fakeSMTPServer) wait(t *testing.T) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(time.Second):
		t.Fatal("smtp server did not finish")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		t.Fatal(s.err)
	}
}

func (s *fakeSMTPServer) setErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *fakeSMTPServer) serve() {
	defer close(s.done)
	conn, err := s.listener.Accept()
	if err != nil {
		if !errors.Is(err, net.ErrClosed) {
			s.setErr(err)
		}
		return
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	if err := writeSMTPLine(writer, "220 fake smtp\r\n"); err != nil {
		s.setErr(err)
		return
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.setErr(err)
			}
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if s.override != nil {
			if response, ok := s.override(line); ok {
				if strings.HasPrefix(line, "RCPT TO:") {
					s.recordRCPT(line, response)
				}
				if err := writeSMTPLine(writer, response); err != nil {
					s.setErr(err)
					return
				}
				continue
			}
		}
		switch {
		case strings.HasPrefix(line, "EHLO "):
			if err := writeSMTPLine(writer, "250-fake\r\n250 OK\r\n"); err != nil {
				s.setErr(err)
				return
			}
		case strings.HasPrefix(line, "MAIL FROM:"):
			s.mu.Lock()
			s.mailFrom = trimSMTPPath(strings.TrimPrefix(line, "MAIL FROM:"))
			s.mu.Unlock()
			if err := writeSMTPLine(writer, "250 OK\r\n"); err != nil {
				s.setErr(err)
				return
			}
		case strings.HasPrefix(line, "RCPT TO:"):
			s.recordRCPT(line, "250 OK\r\n")
			if err := writeSMTPLine(writer, "250 OK\r\n"); err != nil {
				s.setErr(err)
				return
			}
		case line == "DATA":
			if err := writeSMTPLine(writer, "354 End data\r\n"); err != nil {
				s.setErr(err)
				return
			}
			data, err := readSMTPData(reader)
			if err != nil {
				s.setErr(err)
				return
			}
			s.mu.Lock()
			s.data = data
			s.mu.Unlock()
			if err := writeSMTPLine(writer, "250 OK\r\n"); err != nil {
				s.setErr(err)
				return
			}
		case line == "QUIT":
			_ = writeSMTPLine(writer, "221 Bye\r\n")
			return
		default:
			if err := writeSMTPLine(writer, "250 OK\r\n"); err != nil {
				s.setErr(err)
				return
			}
		}
	}
}

func (s *fakeSMTPServer) recordRCPT(line string, response string) {
	recipient := trimSMTPPath(strings.TrimPrefix(line, "RCPT TO:"))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rcptTo = append(s.rcptTo, recipient)
	if smtpResponseAccepted(response) {
		s.acceptedRcptTo = append(s.acceptedRcptTo, recipient)
		return
	}
	s.refusedRcptTo = append(s.refusedRcptTo, recipient)
}

func smtpResponseAccepted(response string) bool {
	response = strings.TrimSpace(response)
	return strings.HasPrefix(response, "250") || strings.HasPrefix(response, "251")
}

func writeSMTPLine(writer *bufio.Writer, line string) error {
	if _, err := writer.WriteString(line); err != nil {
		return err
	}
	return writer.Flush()
}

func readSMTPData(reader *bufio.Reader) (string, error) {
	var data strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		if line == ".\r\n" || line == ".\n" {
			return data.String(), nil
		}
		data.WriteString(line)
	}
}

func trimSMTPPath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "<")
	value = strings.TrimSuffix(value, ">")
	return value
}
