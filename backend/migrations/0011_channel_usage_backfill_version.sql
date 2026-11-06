ALTER TABLE channel_usage_sync_states
    ADD COLUMN IF NOT EXISTS backfill_version INTEGER NOT NULL DEFAULT 0;
