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

func parseURL(rawURL string) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, errors.New("empty url")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	if parsed.Host == "" {
		return nil, errors.New("missing host")
	}
	return parsed, nil
}

func normalizeDomain(rawURL string) string {
	parsed, err := parseURL(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func normalizeRootURL(rawURL string) string {
	parsed, err := parseURL(rawURL)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" {
		return strings.ToLower(parsed.Host)
	}
	return fmt.Sprintf("%s://%s", strings.ToLower(parsed.Scheme), strings.ToLower(parsed.Host))
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
        root_url AS "rootURL",
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

	sql := `
WITH inserted AS (
    INSERT INTO browser_history (url, title, domain, root_url, visited_at, notes, visit_count)
    VALUES ($1, $2, $3, $4, $5, $6, GREATEST($7, 1))
    RETURNING *
)
SELECT row_to_json(t)
FROM (
    SELECT
        id,
        url,
        title,
        domain,
        root_url AS "rootURL",
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
) t;`

	out, err := r.db.Query(ctx, sql,
		payload.URL,
		payload.Title,
		normalizeDomain(payload.URL),
		normalizeRootURL(payload.URL),
		visitedAt,
		payload.Notes,
		payload.VisitCount,
	)
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

	type batchRecord struct {
		URL        string    `json:"url"`
		Title      string    `json:"title"`
		Domain     string    `json:"domain"`
		RootURL    string    `json:"root_url"`
		VisitedAt  time.Time `json:"visited_at"`
		Notes      string    `json:"notes"`
		VisitCount int       `json:"visit_count"`
	}

	records := make([]batchRecord, 0, len(payload.Records))
	for _, item := range payload.Records {
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		visitedAt, err := parseVisitedAt(item.VisitedAt)
		if err != nil {
			return 0, err
		}
		records = append(records, batchRecord{
			URL:        item.URL,
			Title:      item.Title,
			Domain:     normalizeDomain(item.URL),
			RootURL:    normalizeRootURL(item.URL),
			VisitedAt:  visitedAt,
			Notes:      item.Notes,
			VisitCount: max(item.VisitCount, 1),
		})
	}

	if len(records) == 0 {
		return 0, nil
	}

	jsonPayload, err := json.Marshal(records)
	if err != nil {
		return 0, fmt.Errorf("marshal batch records: %w", err)
	}

	const sql = `
WITH payload AS (
    SELECT *
    FROM jsonb_to_recordset($1::jsonb) AS x(
        url text,
        title text,
        domain text,
        root_url text,
        visited_at timestamptz,
        notes text,
        visit_count integer
    )
), upserted AS (
    INSERT INTO browser_history (url, title, domain, root_url, visited_at, notes, visit_count)
    SELECT url, title, domain, root_url, visited_at, notes, GREATEST(visit_count, 1)
    FROM payload
    ON CONFLICT (url, visited_at) DO UPDATE
    SET title = EXCLUDED.title,
        domain = EXCLUDED.domain,
        root_url = EXCLUDED.root_url,
        notes = EXCLUDED.notes,
        visit_count = GREATEST(browser_history.visit_count, EXCLUDED.visit_count),
        updated_at = NOW()
    RETURNING 1
)
SELECT COUNT(*)::text FROM upserted;`

	out, err := r.db.Query(ctx, sql, jsonPayload)
	if err != nil {
		return 0, err
	}

	var inserted int
	if _, err := fmt.Sscanf(string(out), "%d", &inserted); err != nil {
		return 0, fmt.Errorf("parse batch upsert count: %w", err)
	}
	return inserted, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (Record, error) {
	items, err := r.fetchRecords(ctx, "WHERE id = $1", "", "", id)
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

	sql := `
WITH updated AS (
    UPDATE browser_history
    SET url = $1,
        title = $2,
        domain = $3,
        root_url = $4,
        visited_at = $5,
        notes = $6,
        visit_count = GREATEST($7, 1),
        updated_at = NOW()
    WHERE id = $8
    RETURNING *
)
SELECT COALESCE(row_to_json(t), '{}'::json)
FROM (
    SELECT
        id,
        url,
        title,
        domain,
        root_url AS "rootURL",
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
) t;`

	out, err := r.db.Query(ctx, sql,
		payload.URL,
		payload.Title,
		normalizeDomain(payload.URL),
		normalizeRootURL(payload.URL),
		visitedAt,
		payload.Notes,
		payload.VisitCount,
		id,
	)
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
	const sql = `SELECT COUNT(*) FROM (DELETE FROM browser_history WHERE id = $1 RETURNING 1) t;`
	out, err := r.db.Query(ctx, sql, id)
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
	args := make([]any, 0, 3)
	if trimmed := strings.TrimSpace(search); trimmed != "" {
		whereClause = "WHERE url ILIKE $1 OR title ILIKE $1 OR domain ILIKE $1 OR root_url ILIKE $1 OR notes ILIKE $1"
		args = append(args, "%"+trimmed+"%")
	}

	items, err := r.fetchRecords(ctx, whereClause, "ORDER BY visited_at DESC, id DESC", fmt.Sprintf("LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2), append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}

	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM browser_history %s;`, whereClause)
	out, err := r.db.Query(ctx, countSQL, args...)
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
	return r.fetchRecords(ctx, "WHERE recent_rank <= $1", "ORDER BY visited_at DESC, id DESC", "", limit)
}

func (r *Repository) ListRootURLStats(ctx context.Context, days, limit int) ([]RootURLStat, error) {
	sql := `
WITH recent_history AS (
    SELECT *
    FROM browser_history
    WHERE visited_at >= NOW() - make_interval(days => $1)
      AND root_url <> ''
), ranked_history AS (
    SELECT
        root_url,
        url,
        title,
        domain,
        visited_at,
        visit_count,
        ROW_NUMBER() OVER (PARTITION BY root_url ORDER BY visited_at DESC, id DESC) AS row_num
    FROM recent_history
)
SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json)
FROM (
    SELECT
        root_url AS "rootURL",
        COUNT(*) AS "recordCount",
        COALESCE(SUM(visit_count), 0) AS "visitCountTotal",
        MAX(visited_at) AS "lastVisitedAt",
        MAX(domain) FILTER (WHERE row_num = 1) AS domain,
        MAX(title) FILTER (WHERE row_num = 1) AS "latestTitle",
        MAX(url) FILTER (WHERE row_num = 1) AS "latestURL",
        to_char(MAX(visited_at) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS "displayLastVisitedAt"
    FROM ranked_history
    GROUP BY root_url
    ORDER BY "visitCountTotal" DESC, "recordCount" DESC, "lastVisitedAt" DESC, "rootURL" ASC
    LIMIT $2
) t;`

	out, err := r.db.Query(ctx, sql, days, limit)
	if err != nil {
		return nil, err
	}

	var items []RootURLStat
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) fetchRecords(ctx context.Context, whereClause, orderBy, limitClause string, args ...any) ([]Record, error) {
	sql := buildRecordSelect(whereClause, orderBy, limitClause)
	out, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	var items []Record
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, err
	}
	return items, nil
}
