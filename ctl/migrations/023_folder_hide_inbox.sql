-- A folder can take its letters straight away: they never show in the Inbox list and give no
-- "new mail" notification (the folder still counts its unread letters).
ALTER TABLE folder_rules ADD COLUMN IF NOT EXISTS hide_inbox boolean NOT NULL DEFAULT false;
