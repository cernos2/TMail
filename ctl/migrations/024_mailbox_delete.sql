-- Deleting a mailbox with all its letters: switched off at once, erased after a grace period
-- (the administrator can restore it until then).
ALTER TABLE mailboxes ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE mailboxes ADD COLUMN IF NOT EXISTS purge_after timestamptz;
ALTER TABLE mailboxes ADD COLUMN IF NOT EXISTS deleted_by text NOT NULL DEFAULT '';
