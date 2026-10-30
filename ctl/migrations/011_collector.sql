-- Mail collector: external mailboxes (Gmail, the old .ru server…) fetched into our mailboxes over
-- IMAP (folders and read/flag marks kept) or POP3; optional SMTP to send as that address.
CREATE TABLE external_accounts (
    id             bigserial PRIMARY KEY,
    mailbox        text NOT NULL,                  -- our mailbox the mail goes to
    address        text NOT NULL,                  -- the external e-mail address
    display_name   text NOT NULL DEFAULT '',
    protocol       text NOT NULL DEFAULT 'imap',   -- imap | pop3
    host           text NOT NULL,
    port           integer NOT NULL,
    security       text NOT NULL DEFAULT 'ssl',    -- ssl | starttls | none
    username       text NOT NULL,
    password_enc   bytea NOT NULL,
    smtp_host      text NOT NULL DEFAULT '',
    smtp_port      integer NOT NULL DEFAULT 465,
    smtp_security  text NOT NULL DEFAULT 'ssl',
    smtp_username  text NOT NULL DEFAULT '',
    smtp_password_enc bytea,
    inbox_only     boolean NOT NULL DEFAULT false, -- IMAP: only the Inbox, not all folders
    since          date,                           -- import only letters from this date
    delete_after   boolean NOT NULL DEFAULT false, -- POP3: delete from the server after fetching
    enabled        boolean NOT NULL DEFAULT true,
    status         text NOT NULL DEFAULT 'new',    -- new | importing | ok | error
    last_error     text NOT NULL DEFAULT '',
    last_run       timestamptz,
    imported       integer NOT NULL DEFAULT 0,
    created_by     text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (mailbox, address)
);

CREATE TABLE external_folders (
    account_id  bigint NOT NULL REFERENCES external_accounts(id) ON DELETE CASCADE,
    remote      text NOT NULL,
    uidvalidity bigint NOT NULL,
    last_uid    bigint NOT NULL DEFAULT 0,
    total       integer NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, remote)
);

-- Already fetched letters (Message-ID, or POP3 UIDL): no duplicates, e.g. Gmail labels.
CREATE TABLE external_seen (
    account_id bigint NOT NULL REFERENCES external_accounts(id) ON DELETE CASCADE,
    key        text NOT NULL,
    PRIMARY KEY (account_id, key)
);
