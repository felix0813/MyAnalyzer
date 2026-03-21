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

func TestExpandQuery(t *testing.T) {
	got, err := expandQuery("SELECT $1, $2, $3, $4, $5", "O'Reilly", 42, true, []byte(`{"a":1}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "SELECT 'O''Reilly', 42, TRUE, '{\"a\":1}', NULL"
	if got != want {
		t.Fatalf("unexpected expanded query: %s", got)
	}
}
