-- Interface language and time zone of each user; empty means the server's defaults
-- (DEFAULT_LANG / DEFAULT_TIMEZONE chosen at installation). Set by an administrator or by the user.
ALTER TABLE users ADD COLUMN IF NOT EXISTS lang     text NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS timezone text NOT NULL DEFAULT '';
