-- Background of the webmail: "" (none), a ready-made one by id, or "custom" (own picture below).
ALTER TABLE users ADD COLUMN IF NOT EXISTS background text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS user_backgrounds (
    user_id    bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    data       bytea NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Access tokens for AI agents (MCP). Only the SHA-256 of a token is stored.
CREATE TABLE IF NOT EXISTS mcp_tokens (
    id           bigserial PRIMARY KEY,
    user_id      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL,
    token_hash   text NOT NULL UNIQUE,
    prefix       text NOT NULL,
    can_send     boolean NOT NULL DEFAULT false,
    can_modify   boolean NOT NULL DEFAULT false,
    mailboxes    text[] NOT NULL DEFAULT '{}',   -- empty: every mailbox of the user
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    expires_at   timestamptz,
    revoked_at   timestamptz
);
CREATE INDEX IF NOT EXISTS mcp_tokens_user ON mcp_tokens(user_id);
