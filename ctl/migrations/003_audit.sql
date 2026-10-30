-- Audit trail: sign-ins (including failed ones) and administrator actions.
CREATE TABLE audit_log (
    id          bigserial PRIMARY KEY,
    at          timestamptz NOT NULL DEFAULT now(),
    actor_id    bigint REFERENCES users(id) ON DELETE SET NULL,
    actor_login text NOT NULL DEFAULT '',
    action      text NOT NULL,
    target      text NOT NULL DEFAULT '',
    details     jsonb NOT NULL DEFAULT '{}',
    ip          text NOT NULL DEFAULT ''
);
CREATE INDEX audit_log_at ON audit_log (at DESC);
CREATE INDEX audit_log_action ON audit_log (action, at DESC);

-- A user's own ("personal") mailbox: the user may edit its signature and template.
-- Signatures/templates of shared mailboxes are set by administrators.
ALTER TABLE user_mailboxes ADD COLUMN personal boolean NOT NULL DEFAULT false;

CREATE TABLE mailbox_settings (
    mailbox_id bigint PRIMARY KEY REFERENCES mailboxes(id) ON DELETE CASCADE,
    signature  text NOT NULL DEFAULT '',   -- may use {name} {login} {email}
    template   text NOT NULL DEFAULT '',   -- start text of a new message
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by text NOT NULL DEFAULT ''
);
