ALTER TABLE channel_usage_events ADD COLUMN IF NOT EXISTS cache_known boolean NOT NULL DEFAULT false;
ALTER TABLE relay_usage_events ADD COLUMN IF NOT EXISTS cache_known boolean NOT NULL DEFAULT false;
