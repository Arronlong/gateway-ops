ALTER TABLE relay_stations
    ADD COLUMN IF NOT EXISTS type VARCHAR(16) NOT NULL DEFAULT 'sub2api';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_relay_stations_type'
    ) THEN
        ALTER TABLE relay_stations
            ADD CONSTRAINT chk_relay_stations_type CHECK (type IN ('sub2api', 'newapi'));
    END IF;
END $$;
