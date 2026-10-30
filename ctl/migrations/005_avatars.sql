-- Profile photos uploaded by users (normalised 160x160 JPEG) and a cache of external avatars.
CREATE TABLE user_avatars (
    user_id    bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    mime       text NOT NULL,
    data       bytea NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- key: "gravatar:<email>" or "bimi:<domain>"; empty data = looked up, nothing found.
CREATE TABLE avatar_cache (
    key        text PRIMARY KEY,
    mime       text NOT NULL DEFAULT '',
    data       bytea NOT NULL DEFAULT '',
    fetched_at timestamptz NOT NULL DEFAULT now()
);
