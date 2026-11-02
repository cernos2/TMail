-- "Send later": the letter waits in the folder Scheduled and is sent at its time by the server
-- (the browser may be closed). "Snooze": a letter leaves the inbox and comes back at the chosen time.
CREATE TABLE IF NOT EXISTS scheduled_messages (
    id          bigserial PRIMARY KEY,
    user_id     bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account     text NOT NULL,                -- mailbox it is sent from
    ident_id    bigint,                       -- external address with its own SMTP (mail collector), if any
    message_id  text NOT NULL,
    subject     text NOT NULL DEFAULT '',
    recipients  text[] NOT NULL,              -- To, Cc and Bcc
    reply_folder text NOT NULL DEFAULT '',
    reply_uid   text NOT NULL DEFAULT '',
    send_at     timestamptz NOT NULL,
    status      text NOT NULL DEFAULT 'pending', -- pending | sending | sent | failed | cancelled
    error       text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    sent_at     timestamptz
);
CREATE INDEX IF NOT EXISTS scheduled_due ON scheduled_messages (send_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS scheduled_account ON scheduled_messages (account, message_id);

CREATE TABLE IF NOT EXISTS snoozed_messages (
    id         bigserial PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account    text NOT NULL,
    message_id text NOT NULL,
    folder     text NOT NULL,       -- where it comes back to
    until      timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (account, message_id)
);
CREATE INDEX IF NOT EXISTS snoozed_due ON snoozed_messages (until);
