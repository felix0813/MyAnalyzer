package history

import "testing"

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
	tests := map[string]string{
		"https://example.com/path/to/page?foo=bar": "https://example.com",
		"http://Sub.Example.com/a/b":               "http://sub.example.com",
		"example.com/path?foo=bar":                 "example.com",
	}

	for input, want := range tests {
		if got := normalizeRootURL(input); got != want {
			t.Fatalf("normalizeRootURL(%q) = %q, want %q", input, got, want)
		}
	}
}
