package config

import "os"

type Config struct {
	Environment    string
	HTTPAddr       string
	DatabaseURL    string
	OdooSourceRoot string
}

func Load() Config {
	return Config{
		Environment:    env("GORP_ENV", "development"),
		HTTPAddr:       env("GORP_HTTP_ADDR", ":8069"),
		DatabaseURL:    env("GORP_DATABASE_URL", ""),
		OdooSourceRoot: env("GORP_ODOO_SOURCE_ROOT", "/Users/fadhelalqaidoom/Desktop/odoo"),
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
