ALTER TABLE channels ADD COLUMN IF NOT EXISTS pelican_key_cipher text NOT NULL DEFAULT '';
ALTER TABLE relay_pelican_configs ADD COLUMN IF NOT EXISTS channel_id bigint NOT NULL DEFAULT 0;
