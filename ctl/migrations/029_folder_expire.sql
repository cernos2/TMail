-- A folder may delete its letters N days after they arrived (0 = keep). They go to Trash, which
-- Dovecot empties after 30 days.
ALTER TABLE folder_rules ADD COLUMN IF NOT EXISTS expire_days int NOT NULL DEFAULT 0;
