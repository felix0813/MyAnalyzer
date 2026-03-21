package database

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInitSQLUsesURLHashUniqueIndexForBrowserHistory(t *testing.T) {
	sql, err := ReadSQLFile(filepath.Join("..", "..", "init.sql"))
	if err != nil {
		t.Fatalf("read init.sql: %v", err)
	}

	requiredSnippets := []string{
		"CREATE EXTENSION IF NOT EXISTS pgcrypto;",
		"url_hash    TEXT        GENERATED ALWAYS AS (encode(digest(url, 'sha256'), 'hex')) STORED,",
		"ADD COLUMN IF NOT EXISTS url_hash TEXT GENERATED ALWAYS AS (encode(digest(url, 'sha256'), 'hex')) STORED;",
		"ON browser_history (url_hash, visited_at);",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(sql, snippet) {
			t.Fatalf("expected init.sql to contain %q", snippet)
		}
	}

	if strings.Contains(sql, "UNIQUE (url, visited_at)") {
		t.Fatalf("init.sql still contains raw url uniqueness constraint")
	}
}
