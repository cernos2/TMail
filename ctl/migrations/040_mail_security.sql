-- Sending limits per mailbox (protection against spam from a stolen password): NULL = the
-- server-wide limit, 0 = no limit. Postfix asks TMAIL before accepting each letter.
ALTER TABLE mailboxes ADD COLUMN IF NOT EXISTS limit_hour int CHECK (limit_hour >= 0);
ALTER TABLE mailboxes ADD COLUMN IF NOT EXISTS limit_day int CHECK (limit_day >= 0);

CREATE TABLE IF NOT EXISTS send_log (
    mailbox    text NOT NULL,
    at         timestamptz NOT NULL DEFAULT now(),
    recipients int NOT NULL DEFAULT 1,
    blocked    boolean NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS send_log_mailbox_at ON send_log (mailbox, at);
