-- Address book built from correspondence: who a mailbox wrote to (Sent) and who wrote to it (INBOX).
CREATE TABLE contacts (
    mailbox_id bigint NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
    email      text NOT NULL CHECK (email = lower(email)),
    name       text NOT NULL DEFAULT '',
    sent_count int NOT NULL DEFAULT 0,   -- messages this mailbox sent to the address
    recv_count int NOT NULL DEFAULT 0,   -- messages received from the address
    last_at    timestamptz NOT NULL DEFAULT now(),
    search     text NOT NULL DEFAULT '',  -- lowercased "name words + email" for suggestions
    PRIMARY KEY (mailbox_id, email)
);

-- Incremental sync position per mailbox folder (UIDVALIDITY + last seen UID).
CREATE TABLE contact_sync (
    mailbox_id     bigint PRIMARY KEY REFERENCES mailboxes(id) ON DELETE CASCADE,
    inbox_validity bigint NOT NULL DEFAULT 0,
    inbox_uid      bigint NOT NULL DEFAULT 0,
    sent_validity  bigint NOT NULL DEFAULT 0,
    sent_uid       bigint NOT NULL DEFAULT 0,
    synced_at      timestamptz NOT NULL DEFAULT now()
);
