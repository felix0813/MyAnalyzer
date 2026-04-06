package history

import (
	"testing"
	"time"
)

func TestParseIDFromPath(t *testing.T) {
	id, err := parseIDFromPath("/api/history/records/42", "/api/history/records/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Fatalf("expected 42, got %d", id)
	}
}

func TestParseIDFromPathRejectsNestedRoutes(t *testing.T) {
	if _, err := parseIDFromPath("/api/history/records/42/edit", "/api/history/records/"); err == nil {
		t.Fatal("expected error for nested route")
	}
}

func TestParseInt(t *testing.T) {
	if got := parseInt("999", 20, 1, 100); got != 100 {
		t.Fatalf("expected 100, got %d", got)
	}
	if got := parseInt("-1", 20, 1, 100); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
	if got := parseInt("abc", 20, 1, 100); got != 20 {
		t.Fatalf("expected fallback 20, got %d", got)
	}
}

func TestNormalizeRootURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "path and query trimmed", raw: "https://Example.com/news/world?id=1", want: "https://example.com"},
		{name: "port preserved", raw: "http://Example.com:8080/path/to/page?debug=1", want: "http://example.com:8080"},
		{name: "invalid url returns empty", raw: "not a url", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRootURL(tt.raw); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestParseRFC3339Optional(t *testing.T) {
	got, err := parseRFC3339Optional("2026-04-06T10:30:00+08:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected parsed time, got nil")
	}
	if got.Format(time.RFC3339) != "2026-04-06T02:30:00Z" {
		t.Fatalf("unexpected UTC conversion: %s", got.Format(time.RFC3339))
	}
}

func TestParseRFC3339OptionalEmpty(t *testing.T) {
	got, err := parseRFC3339Optional("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil time, got %v", *got)
	}
}

func TestParseRFC3339OptionalRejectsInvalid(t *testing.T) {
	if _, err := parseRFC3339Optional("2026-04-06"); err == nil {
		t.Fatal("expected parsing error for non-RFC3339 format")
	}
}
