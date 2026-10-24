CREATE TABLE IF NOT EXISTS relay_group_pelican_configs (
 id bigserial PRIMARY KEY,
 relay_station_id bigint NOT NULL,
 group_external_id bigint NOT NULL,
 enabled boolean NOT NULL DEFAULT false,
 cron_expression varchar(128) NOT NULL,
 next_run_at timestamptz NOT NULL,
 lease_until timestamptz,
 created_at timestamptz,
 updated_at timestamptz,
 CONSTRAINT idx_group_pelican_target UNIQUE(relay_station_id,group_external_id)
);
CREATE INDEX IF NOT EXISTS idx_relay_group_pelican_configs_next_run_at ON relay_group_pelican_configs(next_run_at);
CREATE TABLE IF NOT EXISTS relay_group_pelican_results (
 id bigserial PRIMARY KEY,
 relay_station_id bigint NOT NULL,
 group_external_id bigint NOT NULL,
 group_name varchar(256) NOT NULL,
 model varchar(256) NOT NULL,
 prompt text NOT NULL,
 reasoning_effort varchar(16) NOT NULL,
 trigger varchar(16) NOT NULL,
 status varchar(16) NOT NULL,
 html text NOT NULL,
 error text NOT NULL,
 duration_ms bigint NOT NULL,
 started_at timestamptz NOT NULL,
 finished_at timestamptz NOT NULL,
 upstream_group_name varchar(256),
 upstream_group_multiplier double precision
);
CREATE INDEX IF NOT EXISTS idx_group_pelican_history ON relay_group_pelican_results(relay_station_id,group_external_id,finished_at DESC);
