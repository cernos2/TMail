-- Large files sent as links: stored in S3 (Garage), served at https://files.<domain>/f/<token>.
CREATE TABLE big_files (
    id          bigserial PRIMARY KEY,
    token       text NOT NULL UNIQUE,
    s3_key      text NOT NULL,
    name        text NOT NULL,
    mime        text NOT NULL DEFAULT 'application/octet-stream',
    size        bigint NOT NULL,
    thumb       boolean NOT NULL DEFAULT false,
    user_id     bigint REFERENCES users(id) ON DELETE SET NULL,
    mailbox     text NOT NULL DEFAULT '',
    sender_name text NOT NULL DEFAULT '',
    downloads   integer NOT NULL DEFAULT 0,
    last_download timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL
);
CREATE INDEX big_files_expires ON big_files (expires_at);
