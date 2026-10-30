-- Webmail users. A user signs in with a login (not a mailbox address) and may work with
-- several mailboxes; mailboxes keep their own passwords for phones and desktop clients.

CREATE TABLE users (
    id            bigserial PRIMARY KEY,
    login         text NOT NULL UNIQUE CHECK (login = lower(login)),
    display_name  text NOT NULL DEFAULT '',
    password_hash text NOT NULL,                  -- bcrypt
    is_admin      boolean NOT NULL DEFAULT false,
    active        boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Which mailboxes a user works with; can_send = may send as this mailbox.
CREATE TABLE user_mailboxes (
    user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mailbox_id bigint NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
    can_send   boolean NOT NULL DEFAULT true,
    position   int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, mailbox_id)
);

CREATE TABLE sessions (
    id           text PRIMARY KEY,               -- sha256 of the cookie token
    user_id      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    user_agent   text NOT NULL DEFAULT '',
    ip           text NOT NULL DEFAULT ''
);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE user_settings (
    user_id    bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    data       jsonb NOT NULL DEFAULT '{}',
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Who wrote each message sent from the webmail (kept even if the user is deleted).
CREATE TABLE sent_messages (
    id          bigserial PRIMARY KEY,
    user_id     bigint REFERENCES users(id) ON DELETE SET NULL,
    user_login  text NOT NULL,
    mailbox_id  bigint NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
    message_id  text NOT NULL,                   -- without angle brackets
    subject     text NOT NULL DEFAULT '',
    recipients  text[] NOT NULL DEFAULT '{}',
    size_bytes  int NOT NULL DEFAULT 0,
    sent_at     timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX sent_messages_mailbox_msgid ON sent_messages (mailbox_id, message_id);
CREATE INDEX sent_messages_user ON sent_messages (user_id, sent_at DESC);
