CREATE TABLE IF NOT EXISTS task_runs (
    id BIGSERIAL PRIMARY KEY,
    task VARCHAR(48) NOT NULL,
    status VARCHAR(16) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    summary TEXT,
    logs TEXT
);
CREATE INDEX IF NOT EXISTS idx_task_runs_history ON task_runs (task, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_task_runs_status ON task_runs (status);
