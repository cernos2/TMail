-- A contact made on a phone keeps the UID the phone gave it (some phones match contacts by UID).
ALTER TABLE crm_parties ADD COLUMN IF NOT EXISTS dav_uid text;
