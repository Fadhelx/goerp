package audit

import (
	"strings"
	"testing"
)

func TestAuditRedactsSecrets(t *testing.T) {
	log := NewLog()
	log.Append(Event{
		UserID:           1,
		CompanyID:        2,
		AgentID:          3,
		Model:            "mock-chat",
		PermissionResult: "allowed",
		Details: map[string]string{
			"api_key": "sk-secret",
			"note":    "kept",
		},
	})
	events := log.Events()
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Details["api_key"] != "[redacted]" {
		t.Fatalf("details = %+v", events[0].Details)
	}
	if strings.Contains(events[0].Details["api_key"], "sk-secret") || events[0].Details["note"] != "kept" {
		t.Fatalf("details = %+v", events[0].Details)
	}
}
