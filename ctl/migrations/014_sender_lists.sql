-- Sender black / white lists (company-wide), applied by Rspamd when mail arrives.
CREATE TABLE sender_rules (
    id         bigserial PRIMARY KEY,
    kind       text NOT NULL CHECK (kind IN ('block', 'allow')),
    pattern    text NOT NULL,           -- name@domain or @domain
    name       text NOT NULL DEFAULT '', -- display name, for the list
    note       text NOT NULL DEFAULT '',
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (pattern)                     -- a sender is either blocked or trusted, not both
);
