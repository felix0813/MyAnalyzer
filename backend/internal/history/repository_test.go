package history

import (
	"strings"
	"testing"
)

func TestBatchUpsertHistorySQLUsesURLHashConflictTarget(t *testing.T) {
	if !strings.Contains(batchUpsertHistorySQL, "ON CONFLICT (url_hash, visited_at)") {
		t.Fatalf("expected batch upsert SQL to target url_hash uniqueness, got:\n%s", batchUpsertHistorySQL)
	}

	if strings.Contains(batchUpsertHistorySQL, "ON CONFLICT (url, visited_at)") {
		t.Fatalf("batch upsert SQL still targets raw url uniqueness, got:\n%s", batchUpsertHistorySQL)
	}
}
