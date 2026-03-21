package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"myanalyzer/backend/internal/database"
)

var errNotFound = errors.New("record not found")

type Repository struct {
	db *database.Client
}

func NewRepository(db *database.Client) *Repository {
	return &Repository{db: db}
}

func parseVisitedAt(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Now().UTC(), nil
	}
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("visitedAt must be RFC3339: %w", err)
	}
	return ts.UTC(), nil
}

func normalizeDomain(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func buildRecordSelect(whereClause string, orderBy string, limitClause string) string {
	return fmt.Sprintf(`
SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json)
FROM (
    SELECT
        id,
        url,
        title,
        domain,
        visited_at AS "visitedAt",
        notes,
        visit_count AS "visitCount",
        created_at AS "createdAt",
        updated_at AS "updatedAt",
        display_title AS "displayTitle",
        display_visited_at AS "displayVisitedAt",
        display_visited_date AS "displayVisitedDate",
        display_visited_time AS "displayVisitedTime"
    FROM v_browser_history_client
    %s
    %s
    %s
) t;`, whereClause, orderBy, limitClause)
}

func (r *Repository) Create(ctx context.Context, payload recordPayload) (Record, error) {
	visitedAt, err := parseVisitedAt(payload.VisitedAt)
	if err != nil {
		return Record{}, err
	}

	sql := fmt.Sprintf(`
WITH inserted AS (
    INSERT INTO browser_history (url, title, domain, visited_at, notes, visit_count)
    VALUES (%s, %s, %s, %s, %s, GREATEST(%d, 1))
    RETURNING *
)
SELECT row_to_json(t)
FROM (
    SELECT
        id,
        url,
        title,
        domain,
        visited_at AS "visitedAt",
        notes,
        visit_count AS "visitCount",
        created_at AS "createdAt",
        updated_at AS "updatedAt",
        COALESCE(NULLIF(title, ''), NULLIF(domain, ''), url) AS "displayTitle",
        to_char(visited_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS "displayVisitedAt",
        to_char(visited_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS "displayVisitedDate",
        to_char(visited_at AT TIME ZONE 'UTC', 'HH24:MI:SS') AS "displayVisitedTime"
    FROM inserted
) t;`,
		database.Quote(payload.URL),
		database.Quote(payload.Title),
		database.Quote(normalizeDomain(payload.URL)),
		database.Quote(visitedAt.Format(time.RFC3339)),
		database.Quote(payload.Notes),
		payload.VisitCount,
	)

	out, err := r.db.Query(ctx, sql)
	if err != nil {
		return Record{}, err
	}

	var record Record
	if err := json.Unmarshal(out, &record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (r *Repository) CreateBatch(ctx context.Context, payload batchPayload) (int, error) {
	if len(payload.Records) == 0 {
		return 0, nil
	}

	statements := make([]string, 0, len(payload.Records))
	inserted := 0
	for _, item := range payload.Records {
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		visitedAt, err := parseVisitedAt(item.VisitedAt)
		if err != nil {
			return 0, err
		}
		statements = append(statements, fmt.Sprintf(`
INSERT INTO browser_history (url, title, domain, visited_at, notes, visit_count)
VALUES (%s, %s, %s, %s, %s, GREATEST(%d, 1))
ON CONFLICT (url, visited_at) DO UPDATE
SET title = EXCLUDED.title,
    domain = EXCLUDED.domain,
    notes = EXCLUDED.notes,
    visit_count = GREATEST(browser_history.visit_count, EXCLUDED.visit_count),
    updated_at = NOW();`,
			database.Quote(item.URL),
			database.Quote(item.Title),
			database.Quote(normalizeDomain(item.URL)),
			database.Quote(visitedAt.Format(time.RFC3339)),
			database.Quote(item.Notes),
			max(item.VisitCount, 1),
		))
		inserted++
	}

	if len(statements) == 0 {
		return 0, nil
	}

	if err := r.db.Exec(ctx, "BEGIN;\n"+strings.Join(statements, "\n")+"\nCOMMIT;"); err != nil {
		return 0, err
	}
	return inserted, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (Record, error) {
	items, err := r.fetchRecords(ctx, fmt.Sprintf("WHERE id = %d", id), "", "")
	if err != nil {
		return Record{}, err
	}
	if len(items) == 0 {
		return Record{}, errNotFound
	}
	return items[0], nil
}

func (r *Repository) Update(ctx context.Context, id int64, payload recordPayload) (Record, error) {
	visitedAt, err := parseVisitedAt(payload.VisitedAt)
	if err != nil {
		return Record{}, err
	}

	sql := fmt.Sprintf(`
WITH updated AS (
    UPDATE browser_history
    SET url = %s,
        title = %s,
        domain = %s,
        visited_at = %s,
        notes = %s,
        visit_count = GREATEST(%d, 1),
        updated_at = NOW()
    WHERE id = %d
    RETURNING *
)
SELECT COALESCE(row_to_json(t), '{}'::json)
FROM (
    SELECT
        id,
        url,
        title,
        domain,
        visited_at AS "visitedAt",
        notes,
        visit_count AS "visitCount",
        created_at AS "createdAt",
        updated_at AS "updatedAt",
        COALESCE(NULLIF(title, ''), NULLIF(domain, ''), url) AS "displayTitle",
        to_char(visited_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS "displayVisitedAt",
        to_char(visited_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS "displayVisitedDate",
        to_char(visited_at AT TIME ZONE 'UTC', 'HH24:MI:SS') AS "displayVisitedTime"
    FROM updated
) t;`,
		database.Quote(payload.URL),
		database.Quote(payload.Title),
		database.Quote(normalizeDomain(payload.URL)),
		database.Quote(visitedAt.Format(time.RFC3339)),
		database.Quote(payload.Notes),
		payload.VisitCount,
		id,
	)

	out, err := r.db.Query(ctx, sql)
	if err != nil {
		return Record{}, err
	}
	if string(out) == "{}" {
		return Record{}, errNotFound
	}

	var record Record
	if err := json.Unmarshal(out, &record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	sql := fmt.Sprintf(`SELECT COUNT(*) FROM (DELETE FROM browser_history WHERE id = %d RETURNING 1) t;`, id)
	out, err := r.db.Query(ctx, sql)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) == "0" {
		return errNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, limit, offset int, search string) ([]Record, int, error) {
	whereClause := ""
	if trimmed := strings.TrimSpace(search); trimmed != "" {
		q := database.Quote("%" + trimmed + "%")
		whereClause = fmt.Sprintf("WHERE url ILIKE %s OR title ILIKE %s OR domain ILIKE %s OR notes ILIKE %s", q, q, q, q)
	}

	items, err := r.fetchRecords(ctx, whereClause, "ORDER BY visited_at DESC, id DESC", fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset))
	if err != nil {
		return nil, 0, err
	}

	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM browser_history %s;`, strings.Replace(whereClause, "title", "title", 1))
	out, err := r.db.Query(ctx, countSQL)
	if err != nil {
		return nil, 0, err
	}

	var total int
	if _, err := fmt.Sscanf(string(out), "%d", &total); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *Repository) ListRecent(ctx context.Context, limit int) ([]Record, error) {
	return r.fetchRecords(ctx, fmt.Sprintf("WHERE recent_rank <= %d", limit), "ORDER BY visited_at DESC, id DESC", "")
}

func (r *Repository) fetchRecords(ctx context.Context, whereClause, orderBy, limitClause string) ([]Record, error) {
	sql := buildRecordSelect(whereClause, orderBy, limitClause)
	out, err := r.db.Query(ctx, sql)
	if err != nil {
		return nil, err
	}

	var items []Record
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
