-- Letters held back by the antivirus (collector imports; SMTP malware is refused by Rspamd).
-- The letter itself lives in S3 under s3_key; removed after 30 days.
CREATE TABLE IF NOT EXISTS av_quarantine (
    id          bigserial PRIMARY KEY,
    mailbox     text NOT NULL,
    source      text NOT NULL,
    sender      text NOT NULL DEFAULT '',
    subject     text NOT NULL DEFAULT '',
    virus       text NOT NULL,
    size        bigint NOT NULL DEFAULT 0,
    s3_key      text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz,
    released_by text
);
CREATE INDEX IF NOT EXISTS av_quarantine_created ON av_quarantine(created_at);
