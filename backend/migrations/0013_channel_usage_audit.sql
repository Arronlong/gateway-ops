ALTER TABLE channel_usage_sync_states ADD COLUMN IF NOT EXISTS history_checked_at TIMESTAMPTZ;
ALTER TABLE channel_usage_sync_states ADD COLUMN IF NOT EXISTS history_audit_before TIMESTAMPTZ;
ALTER TABLE channel_usage_sync_states ADD COLUMN IF NOT EXISTS history_audit_remaining BIGINT;
