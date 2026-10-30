-- Indexes for what grows with the mail: searches by part of a word, links between cards,
-- the antivirus and sending statistics of the last day.

-- "%daisy%" in contact autocomplete and the counterparty search: a trigram index, not a full scan.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS crm_parties_search_trgm ON crm_parties USING gin (search gin_trgm_ops);
CREATE INDEX IF NOT EXISTS contacts_search_trgm ON contacts USING gin (search gin_trgm_ops);

-- The same letter in two cards (person ↔ company, company ↔ company link counts) and a mailbox purge.
CREATE INDEX IF NOT EXISTS crm_letters_message ON crm_letters (account, message_id);
-- Merging cards moves the letters analysed for one card to the other.
CREATE INDEX IF NOT EXISTS crm_processed_party ON crm_processed (party_id) WHERE party_id IS NOT NULL;

-- "Server" page: files checked and letters sent in the last 24 hours.
CREATE INDEX IF NOT EXISTS av_scans_scanned ON av_scans (scanned_at);
CREATE INDEX IF NOT EXISTS sent_messages_sent ON sent_messages (sent_at);
