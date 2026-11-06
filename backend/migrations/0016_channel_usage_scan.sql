ALTER TABLE channel_usage_sync_states ADD COLUMN IF NOT EXISTS history_scan TEXT NOT NULL DEFAULT '';
