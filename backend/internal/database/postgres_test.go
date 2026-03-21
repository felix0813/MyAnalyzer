package database

import "testing"

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig("postgres://postgres:secret@db.example.com:5433/myanalyzer?sslmode=disable")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.host != "db.example.com" || cfg.port != "5433" || cfg.user != "postgres" || cfg.password != "secret" || cfg.database != "myanalyzer" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestParseConfigRejectsUnsupportedSSLMode(t *testing.T) {
	if _, err := parseConfig("postgres://postgres:secret@127.0.0.1:5432/myanalyzer?sslmode=require"); err == nil {
		t.Fatal("expected sslmode error")
	}
}
