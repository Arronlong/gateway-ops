ALTER TABLE channel_usage_sync_states ADD COLUMN IF NOT EXISTS history_verified_until TIMESTAMPTZ;
ALTER TABLE channel_usage_sync_states ADD COLUMN IF NOT EXISTS history_resume TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS channel_usage_history_seen (
 run_id VARCHAR(64) NOT NULL,
 remote_usage_id VARCHAR(256) NOT NULL,
 identity VARCHAR(256) NOT NULL,
 page_identity VARCHAR(256) NOT NULL DEFAULT '',
 occurrence INTEGER NOT NULL,
 PRIMARY KEY (run_id, remote_usage_id)
);
CREATE INDEX IF NOT EXISTS idx_usage_history_identity ON channel_usage_history_seen(run_id, identity);
CREATE INDEX IF NOT EXISTS idx_usage_history_page ON channel_usage_history_seen(run_id, page_identity);
