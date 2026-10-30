-- Background jobs that must outlive a restart of the server (the CRM re-analysis): what is
-- left to do, removed when the job ends.
CREATE TABLE IF NOT EXISTS app_state (
    key        text PRIMARY KEY,
    value      jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
