-- App passwords: a separate password for each mail program or phone, revocable in settings.
-- Mail programs sign in with a mailbox address (IMAP, POP3, SMTP); calendars and contacts
-- (CalDAV, CardDAV) with the TMAIL login or a mailbox address.
CREATE TABLE IF NOT EXISTS app_passwords (
    id            bigserial PRIMARY KEY,
    user_id       bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          text NOT NULL,
    prefix        text NOT NULL UNIQUE,         -- first 8 characters: finds the row without trying every hash
    password_hash text NOT NULL,                -- BLF-CRYPT, as Dovecot expects
    mailboxes     text[] NOT NULL DEFAULT '{}', -- empty: every mailbox of the user
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_used_at  timestamptz,
    last_ip       text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS app_passwords_user ON app_passwords (user_id);

-- Dovecot's lookup for an app password (third passdb). Runs as the owner, so the read-only
-- MTA role can note when the password was last used, but sees nothing else.
CREATE OR REPLACE FUNCTION mta.app_login(addr text, pfx text, service text, ip text)
RETURNS TABLE ("user" text, password text, userdb_home text, userdb_uid int, userdb_gid int, userdb_quota_rule text)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
    WITH hit AS (
        UPDATE app_passwords a SET last_used_at = now(), last_ip = left(ip, 64)
        FROM users w, user_mailboxes um, mailboxes m, mta.users mu
        WHERE a.prefix = pfx AND w.id = a.user_id AND w.active
          AND um.user_id = w.id AND um.mailbox_id = m.id AND m.address = lower(addr) AND mu.address = m.address
          AND (cardinality(a.mailboxes) = 0 OR m.address = ANY (a.mailboxes))
          AND (service <> 'smtp' OR um.can_send)
        RETURNING m.address, a.password_hash, mu.home, mu.quota_bytes
    )
    SELECT address, password_hash, home, 5000, 5000, '*:bytes=' || quota_bytes FROM hit
$$;
REVOKE ALL ON FUNCTION mta.app_login(text, text, text, text) FROM PUBLIC;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mta_ro') THEN
        GRANT EXECUTE ON FUNCTION mta.app_login(text, text, text, text) TO mta_ro;
    END IF;
END $$;

-- Calendars: every user has a personal one and may create more, share them with colleagues
-- (read or edit) and publish a secret read-only subscription link (ICS) for Google or Apple Calendar.
CREATE TABLE IF NOT EXISTS calendars (
    id         bigserial PRIMARY KEY,
    owner_id   bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       text NOT NULL DEFAULT '',  -- '' = the personal calendar, named in the user's language
    color      text NOT NULL DEFAULT 'blue',
    ics_token  text UNIQUE,               -- subscription link; NULL = not published
    ctag       bigint NOT NULL DEFAULT 1, -- changes with every event: phones re-read only changed calendars
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS calendars_owner ON calendars (owner_id);

CREATE TABLE IF NOT EXISTS calendar_shares (
    calendar_id bigint NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
    user_id     bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    can_write   boolean NOT NULL DEFAULT false,
    hidden      boolean NOT NULL DEFAULT false, -- the colleague switched it off in their calendar
    PRIMARY KEY (calendar_id, user_id)
);
CREATE INDEX IF NOT EXISTS calendar_shares_user ON calendar_shares (user_id);
ALTER TABLE calendars ADD COLUMN IF NOT EXISTS hidden boolean NOT NULL DEFAULT false; -- the owner switched it off

ALTER TABLE cal_events ADD COLUMN IF NOT EXISTS calendar_id bigint REFERENCES calendars(id) ON DELETE CASCADE;
ALTER TABLE cal_events ADD COLUMN IF NOT EXISTS uid text;                       -- iCalendar UID
ALTER TABLE cal_events ADD COLUMN IF NOT EXISTS href text;                      -- file name on the CalDAV server
ALTER TABLE cal_events ADD COLUMN IF NOT EXISTS rrule text NOT NULL DEFAULT ''; -- repeat rule (RFC 5545), '' = once
ALTER TABLE cal_events ADD COLUMN IF NOT EXISTS ical text NOT NULL DEFAULT '';  -- the object as a phone sent it: keeps what TMAIL does not show
ALTER TABLE cal_events ADD COLUMN IF NOT EXISTS remind_at timestamptz;          -- start of the occurrence the next reminder is about (repeating events)

INSERT INTO calendars (owner_id) SELECT id FROM users u WHERE NOT EXISTS (SELECT 1 FROM calendars c WHERE c.owner_id = u.id);
UPDATE cal_events e SET calendar_id = (SELECT min(c.id) FROM calendars c WHERE c.owner_id = e.user_id) WHERE calendar_id IS NULL;
UPDATE cal_events SET uid = gen_random_uuid()::text WHERE uid IS NULL;
UPDATE cal_events SET href = uid || '.ics' WHERE href IS NULL;
ALTER TABLE cal_events ALTER COLUMN calendar_id SET NOT NULL;
ALTER TABLE cal_events ALTER COLUMN uid SET NOT NULL;
ALTER TABLE cal_events ALTER COLUMN href SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS cal_events_href ON cal_events (calendar_id, href);
CREATE INDEX IF NOT EXISTS cal_events_calendar_time ON cal_events (calendar_id, starts_at);

CREATE OR REPLACE FUNCTION cal_bump_ctag() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP <> 'INSERT' THEN
        UPDATE calendars SET ctag = ctag + 1 WHERE id = OLD.calendar_id;
    END IF;
    IF TG_OP <> 'DELETE' AND (TG_OP = 'INSERT' OR NEW.calendar_id <> OLD.calendar_id) THEN
        UPDATE calendars SET ctag = ctag + 1 WHERE id = NEW.calendar_id;
    END IF;
    RETURN NULL;
END $$;
DROP TRIGGER IF EXISTS cal_events_ctag ON cal_events;
CREATE TRIGGER cal_events_ctag AFTER INSERT OR UPDATE OR DELETE ON cal_events FOR EACH ROW EXECUTE FUNCTION cal_bump_ctag();

-- Address book for phones (CardDAV) = counterparties. Phones name new contacts themselves.
ALTER TABLE crm_parties ADD COLUMN IF NOT EXISTS dav_href text UNIQUE;
CREATE TABLE IF NOT EXISTS dav_ctags (name text PRIMARY KEY, ctag bigint NOT NULL DEFAULT 1);
INSERT INTO dav_ctags (name) VALUES ('crm') ON CONFLICT DO NOTHING;
CREATE OR REPLACE FUNCTION crm_bump_ctag() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE dav_ctags SET ctag = ctag + 1 WHERE name = 'crm';
    RETURN NULL;
END $$;
DROP TRIGGER IF EXISTS crm_parties_ctag ON crm_parties;
-- Only what a phone shows: new letters (last_letter) must not make every phone re-read the book.
CREATE TRIGGER crm_parties_ctag AFTER INSERT OR DELETE OR UPDATE OF kind, name, position, notes, website, phones, emails, company_id
    ON crm_parties FOR EACH STATEMENT EXECUTE FUNCTION crm_bump_ctag();
DROP TRIGGER IF EXISTS crm_addresses_ctag ON crm_addresses;
CREATE TRIGGER crm_addresses_ctag AFTER INSERT OR UPDATE OR DELETE ON crm_addresses FOR EACH STATEMENT EXECUTE FUNCTION crm_bump_ctag();
