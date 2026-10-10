-- GatewayOps schema 0003_channel_usage_events
-- 渠道自身 usage/log 持久化。与 relay_usage_events 严格隔离，渠道监控不得读取中转站日志。

CREATE TABLE IF NOT EXISTS channel_usage_events (
    id                       BIGSERIAL PRIMARY KEY,
    channel_id               BIGINT NOT NULL,
    channel_account_id       BIGINT NOT NULL,
    source                   VARCHAR(32) NOT NULL,
    remote_usage_id          VARCHAR(128) NOT NULL,
    request_id               VARCHAR(128),
    model                    VARCHAR(256),
    first_token_ms           BIGINT,
    duration_ms              BIGINT,
    success_known            BOOLEAN NOT NULL DEFAULT FALSE,
    success                  BOOLEAN NOT NULL DEFAULT TRUE,
    input_tokens             BIGINT NOT NULL DEFAULT 0,
    output_tokens            BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens        BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens    BIGINT NOT NULL DEFAULT 0,
    actual_cost              DOUBLE PRECISION NOT NULL DEFAULT 0,
    total_cost               DOUBLE PRECISION NOT NULL DEFAULT 0,
    occurred_at              TIMESTAMPTZ NOT NULL,
    synced_at                TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_usage_remote
    ON channel_usage_events (channel_id, channel_account_id, source, remote_usage_id);

CREATE INDEX IF NOT EXISTS idx_channel_usage_channel_time
    ON channel_usage_events (channel_id, occurred_at);

CREATE INDEX IF NOT EXISTS idx_channel_usage_events_channel_account_id
    ON channel_usage_events (channel_account_id);

CREATE INDEX IF NOT EXISTS idx_channel_usage_events_request_id
    ON channel_usage_events (request_id);

CREATE INDEX IF NOT EXISTS idx_channel_usage_events_synced_at
    ON channel_usage_events (synced_at);

CREATE TABLE IF NOT EXISTS channel_usage_sync_states (
    channel_id          BIGINT NOT NULL,
    channel_account_id  BIGINT NOT NULL,
    forward_synced_at   TIMESTAMPTZ,
    backfilled_at       TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (channel_id, channel_account_id)
);