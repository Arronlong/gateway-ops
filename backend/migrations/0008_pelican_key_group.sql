ALTER TABLE relay_pelican_results ADD COLUMN IF NOT EXISTS upstream_group_name varchar(256) NOT NULL DEFAULT '';
ALTER TABLE relay_pelican_results ADD COLUMN IF NOT EXISTS upstream_group_multiplier double precision;
