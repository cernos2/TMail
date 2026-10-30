-- Antivirus verdicts by file content (sha256), so a file is scanned once, not on every download.
CREATE TABLE IF NOT EXISTS av_scans (
    hash       text PRIMARY KEY,
    found      text NOT NULL DEFAULT '',
    scanned_at timestamptz NOT NULL DEFAULT now()
);
-- Files sent as links: downloadable only once the antivirus has checked them.
ALTER TABLE big_files ADD COLUMN IF NOT EXISTS av_status text NOT NULL DEFAULT 'pending';
ALTER TABLE big_files ADD COLUMN IF NOT EXISTS av_found text NOT NULL DEFAULT '';
-- Pictures from the internet in letters (like Gmail): ask first (default) or always show,
-- plus senders whose pictures are always shown.
ALTER TABLE users ADD COLUMN IF NOT EXISTS image_policy text NOT NULL DEFAULT 'ask';
CREATE TABLE IF NOT EXISTS image_senders (
    user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sender     text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, sender)
);
