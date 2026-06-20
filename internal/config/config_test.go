package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("GORP_ENV", "")
	t.Setenv("GORP_HTTP_ADDR", "")
	t.Setenv("GORP_DATABASE_URL", "")
	t.Setenv("GORP_ODOO_SOURCE_ROOT", "")

	cfg := Load()

	if cfg.Environment != "development" {
		t.Fatalf("Environment = %q", cfg.Environment)
	}
	if cfg.HTTPAddr != ":8069" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.OdooSourceRoot == "" {
		t.Fatal("OdooSourceRoot is empty")
	}
}
