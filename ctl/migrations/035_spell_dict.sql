-- Personal spelling dictionary: words a user added from the editor ("Add to dictionary") —
-- names, brands, terms — are not underlined for them any more.
CREATE TABLE IF NOT EXISTS spell_dict (
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    word    text   NOT NULL,
    added   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, word)
);
