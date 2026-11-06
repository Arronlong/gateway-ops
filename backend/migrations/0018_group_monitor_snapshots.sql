CREATE TABLE IF NOT EXISTS relay_group_monitor_days (
 relay_station_id bigint NOT NULL,
 group_external_id bigint NOT NULL,
 day timestamptz NOT NULL,
 source_key varchar(64) NOT NULL,
 payload bytea NOT NULL,
 collected_until timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 PRIMARY KEY (relay_station_id, group_external_id, day)
);
CREATE TABLE IF NOT EXISTS relay_group_monitor_snapshots (
 relay_station_id bigint NOT NULL,
 range_name varchar(16) NOT NULL,
 source_key varchar(64) NOT NULL,
 payload bytea NOT NULL,
 last_error text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL,
 PRIMARY KEY (relay_station_id, range_name)
);
