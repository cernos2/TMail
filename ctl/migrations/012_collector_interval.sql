-- How often each external mailbox is checked, in minutes.
ALTER TABLE external_accounts ADD COLUMN interval_minutes integer NOT NULL DEFAULT 2;
