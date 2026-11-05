ALTER TABLE relay_pelican_results ADD COLUMN IF NOT EXISTS thumbnail BYTEA;
ALTER TABLE relay_group_pelican_results ADD COLUMN IF NOT EXISTS thumbnail BYTEA;
