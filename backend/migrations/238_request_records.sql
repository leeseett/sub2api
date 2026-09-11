-- 238_request_records.sql
-- Full gateway request/response records are kept separately from usage logs so
-- they can be exported for offline analysis without changing billing data.
CREATE TABLE IF NOT EXISTS request_records (
    id BIGSERIAL PRIMARY KEY,
    request_id TEXT,
    client_request_id TEXT,
    user_id BIGINT,
    api_key_id BIGINT,
    account_id BIGINT,
    group_id BIGINT,
    method VARCHAR(16) NOT NULL,
    path TEXT NOT NULL,
    query_string TEXT,
    request_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    request_body BYTEA,
    request_content_type TEXT,
    response_status INTEGER NOT NULL DEFAULT 200,
    response_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_body BYTEA,
    response_content_type TEXT,
    stream BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_request_records_created_at
    ON request_records (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_request_records_request_id
    ON request_records (request_id);
CREATE INDEX IF NOT EXISTS idx_request_records_user_id_created_at
    ON request_records (user_id, created_at DESC, id DESC);
