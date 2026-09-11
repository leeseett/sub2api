-- 239_request_record_metadata.sql
-- Add searchable metadata used by the admin request record view.
ALTER TABLE request_records ADD COLUMN IF NOT EXISTS model TEXT;
ALTER TABLE request_records ADD COLUMN IF NOT EXISTS user_agent TEXT;
ALTER TABLE request_records ADD COLUMN IF NOT EXISTS ip_address TEXT;

CREATE INDEX IF NOT EXISTS idx_request_records_model_created_at
    ON request_records (model, created_at DESC, id DESC);
