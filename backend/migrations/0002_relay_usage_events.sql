-- GatewayOps schema 0002_relay_usage_events
-- 本地持久化 Sub2API usage，供最近调用、稳定性、消费趋势等页面查询。

CREATE TABLE IF NOT EXISTS relay_usage_events (
    id                          BIGSERIAL PRIMARY KEY,
    relay_station_id            BIGINT NOT NULL,
    remote_usage_id             BIGINT NOT NULL,
    account_external_id         BIGINT NOT NULL,
    group_external_id           BIGINT NOT NULL DEFAULT 0,
    group_name                  VARCHAR(256),
    user_email                  VARCHAR(512),
    model                       VARCHAR(256),
    request_type                VARCHAR(64),
    first_token_ms              BIGINT NOT NULL DEFAULT 0,
    duration_ms                 BIGINT NOT NULL DEFAULT 0,
    status_code                 INTEGER NOT NULL DEFAULT 0,
    input_tokens                BIGINT NOT NULL DEFAULT 0,
    output_tokens               BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens           BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens       BIGINT NOT NULL DEFAULT 0,
    cache_creation5m_tokens     BIGINT NOT NULL DEFAULT 0,
    cache_creation1h_tokens     BIGINT NOT NULL DEFAULT 0,
    actual_cost                 DOUBLE PRECISION NOT NULL DEFAULT 0,
    total_cost                  DOUBLE PRECISION NOT NULL DEFAULT 0,
    account_rate_multiplier     DOUBLE PRECISION,
    account_stats_cost          DOUBLE PRECISION,
    occurred_at                 TIMESTAMPTZ NOT NULL,
    synced_at                   TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_relay_usage_remote
    ON relay_usage_events (relay_station_id, remote_usage_id);

CREATE INDEX IF NOT EXISTS idx_relay_usage_station_time
    ON relay_usage_events (relay_station_id, occurred_at);

CREATE INDEX IF NOT EXISTS idx_relay_usage_account_time
    ON relay_usage_events (account_external_id, occurred_at);

CREATE INDEX IF NOT EXISTS idx_relay_usage_events_synced_at
    ON relay_usage_events (synced_at);


CREATE TABLE IF NOT EXISTS relay_usage_sync_states (
    relay_station_id    BIGINT PRIMARY KEY,
    forward_synced_at   TIMESTAMPTZ,
    backfilled_at       TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
