-- Core mail objects. The application owns these tables; Postfix and Dovecot only see views in schema "mta".

CREATE TABLE domains (
    id          bigserial PRIMARY KEY,
    name        text NOT NULL UNIQUE CHECK (name = lower(name)),
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE mailboxes (
    id            bigserial PRIMARY KEY,
    domain_id     bigint NOT NULL REFERENCES domains(id) ON DELETE RESTRICT,
    local_part    text NOT NULL CHECK (local_part = lower(local_part)),
    address       text NOT NULL UNIQUE CHECK (address = lower(address)),
    display_name  text NOT NULL DEFAULT '',
    password_hash text NOT NULL,
    quota_bytes   bigint NOT NULL DEFAULT 5368709120 CHECK (quota_bytes >= 0),
    active        boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (domain_id, local_part)
);

CREATE TABLE aliases (
    id           bigserial PRIMARY KEY,
    domain_id    bigint NOT NULL REFERENCES domains(id) ON DELETE RESTRICT,
    source       text NOT NULL UNIQUE CHECK (source = lower(source)),
    destinations text[] NOT NULL CHECK (cardinality(destinations) > 0),
    active       boolean NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE dkim_keys (
    id          bigserial PRIMARY KEY,
    domain_id   bigint NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    selector    text NOT NULL,
    public_key  text NOT NULL,
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (domain_id, selector)
);

CREATE SCHEMA IF NOT EXISTS mta;

CREATE VIEW mta.domains AS
    SELECT name FROM domains WHERE active;

CREATE VIEW mta.users AS
    SELECT m.address, m.password_hash, m.quota_bytes,
           '/var/vmail/' || d.name || '/' || m.local_part AS home
    FROM mailboxes m JOIN domains d ON d.id = m.domain_id
    WHERE m.active AND d.active;

CREATE VIEW mta.mailboxes AS
    SELECT address FROM mta.users;

CREATE VIEW mta.aliases AS
    SELECT a.source, array_to_string(a.destinations, ',') AS destinations
    FROM aliases a JOIN domains d ON d.id = a.domain_id
    WHERE a.active AND d.active;

-- Which mailbox logins may use a given envelope sender: the mailbox itself,
-- and every local mailbox an alias delivers to (e.g. info@ shared by several people).
CREATE VIEW mta.sender_logins AS
    SELECT sender, string_agg(DISTINCT login, ',') AS logins FROM (
        SELECT address AS sender, address AS login FROM mta.users
        UNION ALL
        SELECT a.source, u.address
        FROM aliases a
        JOIN domains d ON d.id = a.domain_id
        CROSS JOIN LATERAL unnest(a.destinations) AS dst
        JOIN mta.users u ON u.address = dst
        WHERE a.active AND d.active
    ) s
    GROUP BY sender;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mta_ro') THEN
        GRANT USAGE ON SCHEMA mta TO mta_ro;
        GRANT SELECT ON ALL TABLES IN SCHEMA mta TO mta_ro;
    END IF;
END $$;
