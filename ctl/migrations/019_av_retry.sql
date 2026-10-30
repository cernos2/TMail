-- Background antivirus re-checks of file links: attempts and the time of the next one.
ALTER TABLE big_files ADD COLUMN IF NOT EXISTS av_tries int NOT NULL DEFAULT 0;
ALTER TABLE big_files ADD COLUMN IF NOT EXISTS av_next_at timestamptz;
