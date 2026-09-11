-- Keep the common admin filters index-friendly while preserving newest-first
-- pagination. Payload columns are intentionally excluded from these indexes.
CREATE INDEX IF NOT EXISTS idx_request_records_api_key_created_at
    ON request_records (api_key_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_request_records_group_created_at
    ON request_records (group_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_request_records_response_status_created_at
    ON request_records (response_status, created_at DESC, id DESC);
