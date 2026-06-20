package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"version"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Run returned %d", code)
	}
	if !strings.Contains(stdout.String(), "gorp ") {
		t.Fatalf("stdout missing version: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr not empty: %q", stderr.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"missing"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("Run returned %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("stderr missing error: %q", stderr.String())
	}
}

func TestRunModules(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"modules"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Run returned %d stderr=%q", code, stderr.String())
	}
	for _, want := range []string{"oi_base", "oi_workflow", "oi_workflow_advance", "oi_delegation", "oi_login_as"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %s: %q", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr not empty: %q", stderr.String())
	}
}
