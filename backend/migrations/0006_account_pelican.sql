CREATE TABLE IF NOT EXISTS relay_pelican_configs (
 id bigserial PRIMARY KEY,
 relay_station_id bigint NOT NULL,
 account_external_id bigint NOT NULL,
 enabled boolean NOT NULL DEFAULT false,
 model varchar(256) NOT NULL,
 prompt text NOT NULL,
 reasoning_effort varchar(16) NOT NULL,
 parallel_count integer NOT NULL DEFAULT 1,
 cron_expression varchar(128) NOT NULL,
 next_run_at timestamptz NOT NULL,
 lease_until timestamptz,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 CONSTRAINT idx_pelican_account UNIQUE (relay_station_id,account_external_id)
);
CREATE INDEX IF NOT EXISTS idx_relay_pelican_configs_next_run_at ON relay_pelican_configs(next_run_at);
CREATE TABLE IF NOT EXISTS relay_pelican_results (
 id bigserial PRIMARY KEY,
 relay_station_id bigint NOT NULL,
 account_external_id bigint NOT NULL,
 account_name varchar(256) NOT NULL,
 model varchar(256) NOT NULL,
 prompt text NOT NULL,
 reasoning_effort varchar(16) NOT NULL,
 trigger varchar(16) NOT NULL,
 status varchar(16) NOT NULL,
 html text NOT NULL,
 error text NOT NULL,
 duration_ms bigint NOT NULL,
 started_at timestamptz NOT NULL,
 finished_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pelican_history ON relay_pelican_results(relay_station_id,account_external_id,finished_at DESC);
