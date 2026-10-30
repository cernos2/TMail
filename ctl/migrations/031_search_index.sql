-- Search index of letters in the database: subject, addresses, attachment names and the text of
-- every letter as a stemmed full-text vector (word forms match; the text search configuration is set in searchindex.go) plus
-- trigrams of the header line (parts of addresses, numbers). The text itself is not stored.
CREATE TABLE IF NOT EXISTS msg_index (
    account text   NOT NULL,
    folder  text   NOT NULL,
    uid     bigint NOT NULL,
    date    timestamptz,
    hdr     text   NOT NULL,
    tsv     tsvector NOT NULL,
    PRIMARY KEY (account, folder, uid)
);
CREATE INDEX IF NOT EXISTS msg_index_tsv ON msg_index USING gin (tsv);
CREATE INDEX IF NOT EXISTS msg_index_hdr ON msg_index USING gin (hdr gin_trgm_ops);

-- How far each folder is indexed; a new UIDVALIDITY means the folder was rebuilt: index anew.
CREATE TABLE IF NOT EXISTS msg_index_state (
    account     text   NOT NULL,
    folder      text   NOT NULL,
    uidvalidity bigint NOT NULL,
    last_uid    bigint NOT NULL DEFAULT 0,
    complete    boolean NOT NULL DEFAULT false,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account, folder)
);
