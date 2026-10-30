-- "Mail client" mode for external mailboxes: their own mailbox in TMAIL and two-way sync
-- of read / flagged / deleted / archived between TMAIL and the external server.
ALTER TABLE external_accounts ADD COLUMN two_way boolean NOT NULL DEFAULT true;

-- Where each fetched letter lives on the external server, and its last known marks there.
CREATE TABLE external_messages (
    account_id     bigint NOT NULL REFERENCES external_accounts(id) ON DELETE CASCADE,
    remote_folder  text NOT NULL,
    remote_uid     bigint NOT NULL,
    uidvalidity    bigint NOT NULL,
    message_id     text NOT NULL,
    local_folder   text NOT NULL,
    seen           boolean NOT NULL DEFAULT false,
    flagged        boolean NOT NULL DEFAULT false,
    fetched_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, remote_folder, remote_uid)
);
CREATE INDEX external_messages_mid ON external_messages (message_id);

-- External mailboxes: an external address (e.g. name@gmail.com) kept as its own mailbox in
-- TMAIL. Its domain is marked external: Dovecot stores it like any mailbox, but Postfix does not
-- treat the domain as ours (mail to other @gmail.com addresses still goes out normally) and never
-- sends as it — such mail goes through the external server's SMTP.
ALTER TABLE domains ADD COLUMN external boolean NOT NULL DEFAULT false;

CREATE OR REPLACE VIEW mta.domains AS
    SELECT name FROM domains WHERE active AND NOT external;

CREATE OR REPLACE VIEW mta.mailboxes AS
    SELECT m.address FROM mailboxes m JOIN domains d ON d.id = m.domain_id
    WHERE m.active AND d.active AND NOT d.external;

CREATE OR REPLACE VIEW mta.sender_logins AS
    SELECT sender, string_agg(DISTINCT login, ',') AS logins FROM (
        SELECT address AS sender, address AS login FROM mta.mailboxes
        UNION ALL
        SELECT a.source, u.address
        FROM aliases a
        JOIN domains d ON d.id = a.domain_id
        CROSS JOIN LATERAL unnest(a.destinations) AS dst
        JOIN mta.mailboxes u ON u.address = dst
        WHERE a.active AND d.active AND NOT d.external
    ) s GROUP BY sender;

-- Delete from the source server once fetched, keeping the last N days there (like mail clients).
ALTER TABLE external_accounts ADD COLUMN keep_days integer NOT NULL DEFAULT 90;
ALTER TABLE external_seen ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
