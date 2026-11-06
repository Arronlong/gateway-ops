-- 全局代理供所有渠道复用，完整 URL 与认证信息加密存储。
CREATE TABLE IF NOT EXISTS proxy_configs (
    id bigserial PRIMARY KEY,
    display_url varchar(4096) NOT NULL,
    url_cipher text NOT NULL,
    url_hash varchar(64) NOT NULL,
    created_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_proxy_configs_url_hash ON proxy_configs (url_hash);
ALTER TABLE channels ADD COLUMN IF NOT EXISTS proxy_id bigint;
CREATE INDEX IF NOT EXISTS idx_channels_proxy_id ON channels (proxy_id);
