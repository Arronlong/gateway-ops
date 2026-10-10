ALTER TABLE relay_accounts
    ADD COLUMN IF NOT EXISTS identity VARCHAR(256);

CREATE INDEX IF NOT EXISTS idx_relay_accounts_identity
    ON relay_accounts (identity);

CREATE TABLE IF NOT EXISTS channel_relay_account_links (
    id                          BIGSERIAL PRIMARY KEY,
    channel_account_id          BIGINT NOT NULL,
    relay_station_id            BIGINT NOT NULL,
    relay_account_external_id   BIGINT NOT NULL DEFAULT 0,
    mode                        VARCHAR(16) NOT NULL DEFAULT 'auto',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_relay_station
    ON channel_relay_account_links (channel_account_id, relay_station_id);

CREATE INDEX IF NOT EXISTS idx_channel_relay_account_links_channel_account
    ON channel_relay_account_links (channel_account_id);

CREATE INDEX IF NOT EXISTS idx_channel_relay_account_links_station
    ON channel_relay_account_links (relay_station_id);

CREATE INDEX IF NOT EXISTS idx_channel_relay_account_links_external
    ON channel_relay_account_links (relay_account_external_id);

CREATE INDEX IF NOT EXISTS idx_channel_relay_account_links_mode
    ON channel_relay_account_links (mode);
