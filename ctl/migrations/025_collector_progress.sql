-- How much is still waiting on the source server (letters and bytes), for the progress line.
ALTER TABLE external_folders ADD COLUMN IF NOT EXISTS remaining_bytes bigint NOT NULL DEFAULT 0;
