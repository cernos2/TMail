-- The list preview of each letter, kept with its index row: a list of search hits (old letters
-- not in memory) no longer reads the start of every letter from the disk. Filled by the indexer,
-- the rows indexed before this column newest first.
ALTER TABLE msg_index ADD COLUMN IF NOT EXISTS preview text;
