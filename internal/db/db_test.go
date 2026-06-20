package db

import "testing"

func TestOpenRejectsNonPostgres(t *testing.T) {
	if _, err := Open("sqlite", ""); err == nil {
		t.Fatal("expected unsupported driver error")
	}
}
