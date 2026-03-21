CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS browser_history (
    id BIGSERIAL PRIMARY KEY,
    url TEXT NOT NULL,
    root_url TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    domain TEXT NOT NULL DEFAULT '',
    visited_at TIMESTAMPTZ NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    visit_count INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT browser_history_visit_count_positive CHECK (visit_count > 0),
    CONSTRAINT browser_history_url_visited_unique UNIQUE (url, visited_at)
);

ALTER TABLE browser_history
    ADD COLUMN IF NOT EXISTS root_url TEXT NOT NULL DEFAULT '';

UPDATE browser_history
SET root_url = LOWER(
    CASE
        WHEN position('://' IN url) > 0 THEN split_part(url, '/', 3)
        ELSE split_part(split_part(url, '?', 1), '/', 1)
    END
)
WHERE root_url = '';

UPDATE browser_history
SET root_url = LOWER(
    CASE
        WHEN position('://' IN url) > 0 THEN split_part(url, '/', 1) || '//' || split_part(url, '/', 3)
        ELSE split_part(split_part(url, '?', 1), '/', 1)
    END
)
WHERE root_url = LOWER(
    CASE
        WHEN position('://' IN url) > 0 THEN split_part(url, '/', 3)
        ELSE split_part(split_part(url, '?', 1), '/', 1)
    END
);

CREATE INDEX IF NOT EXISTS idx_browser_history_visited_at_desc
    ON browser_history (visited_at DESC);

CREATE INDEX IF NOT EXISTS idx_browser_history_domain_visited_at_desc
    ON browser_history (domain, visited_at DESC);

CREATE INDEX IF NOT EXISTS idx_browser_history_root_url_visited_at_desc
    ON browser_history (root_url, visited_at DESC);

CREATE INDEX IF NOT EXISTS idx_browser_history_search_trgm
    ON browser_history USING GIN ((COALESCE(title, '') || ' ' || COALESCE(url, '') || ' ' || COALESCE(root_url, '') || ' ' || COALESCE(notes, '')) gin_trgm_ops);

CREATE OR REPLACE VIEW v_browser_history_client AS
SELECT
    id,
    url,
    root_url,
    title,
    domain,
    visited_at,
    notes,
    visit_count,
    created_at,
    updated_at,
    COALESCE(NULLIF(title, ''), NULLIF(domain, ''), NULLIF(root_url, ''), url) AS display_title,
    to_char(visited_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS display_visited_at,
    to_char(visited_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS display_visited_date,
    to_char(visited_at AT TIME ZONE 'UTC', 'HH24:MI:SS') AS display_visited_time,
    ROW_NUMBER() OVER (ORDER BY visited_at DESC, id DESC) AS recent_rank
FROM browser_history;

CREATE OR REPLACE VIEW v_browser_history_recent AS
SELECT
    id,
    url,
    root_url,
    title,
    domain,
    visited_at,
    notes,
    visit_count,
    created_at,
    updated_at,
    display_title,
    display_visited_at,
    display_visited_date,
    display_visited_time,
    recent_rank
FROM v_browser_history_client
WHERE recent_rank <= 100
ORDER BY visited_at DESC, id DESC;

CREATE OR REPLACE VIEW v_browser_history_recent_roots AS
SELECT
    root_url,
    MAX(domain) AS domain,
    COUNT(*) AS recent_visit_count,
    MAX(visited_at) AS last_visited_at,
    to_char(MAX(visited_at) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS display_last_visited_at,
    (ARRAY_AGG(COALESCE(NULLIF(title, ''), root_url, url) ORDER BY visited_at DESC, id DESC))[1] AS latest_title,
    (ARRAY_AGG(url ORDER BY visited_at DESC, id DESC))[1] AS latest_url
FROM v_browser_history_recent
WHERE root_url <> ''
GROUP BY root_url
ORDER BY last_visited_at DESC, root_url;
