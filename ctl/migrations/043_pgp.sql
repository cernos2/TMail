-- OpenPGP: a key pair per mailbox (its private part sealed with the server's secret, like the
-- collector passwords) and the public keys of correspondents (imported by hand, from attached
-- keys or from the Autocrypt header of their letters).
CREATE TABLE IF NOT EXISTS pgp_keys (
    mailbox      text PRIMARY KEY,
    fingerprint  text NOT NULL,
    public_key   text NOT NULL,       -- armored
    private_enc  bytea NOT NULL,      -- armored private key, sealed
    sign_default boolean NOT NULL DEFAULT false,
    created_by   text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pgp_contacts (
    email       text PRIMARY KEY CHECK (email = lower(email)),
    fingerprint text NOT NULL,
    public_key  text NOT NULL,
    source      text NOT NULL DEFAULT 'manual', -- manual | attachment | autocrypt
    updated_at  timestamptz NOT NULL DEFAULT now()
);
