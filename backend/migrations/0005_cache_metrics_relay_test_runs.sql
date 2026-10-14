ALTER TABLE channel_usage_events ADD COLUMN IF NOT EXISTS cache_known boolean NOT NULL DEFAULT false;
ALTER TABLE relay_usage_events ADD COLUMN IF NOT EXISTS cache_known boolean NOT NULL DEFAULT false;
CREATE TABLE IF NOT EXISTS relay_test_runs (
 id bigserial PRIMARY KEY,
 relay_station_id bigint NOT NULL,
 station_name varchar(256) NOT NULL,
 target_type varchar(16) NOT NULL,
 target_id bigint NOT NULL,
 target_name varchar(256) NOT NULL,
 model varchar(256) NOT NULL,
 mode varchar(16) NOT NULL,
 requested integer NOT NULL,
 succeeded integer NOT NULL,
 failed integer NOT NULL,
 duration_ms bigint NOT NULL,
 setup_error boolean NOT NULL DEFAULT false,
 error text NOT NULL,
 started_at timestamptz NOT NULL,
 finished_at timestamptz NOT NULL,
 results jsonb NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_relay_test_station_time ON relay_test_runs(relay_station_id, finished_at DESC);
