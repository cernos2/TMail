-- The AI API the assistant uses, chosen in the admin panel (OpenAI-compatible: a local model,
-- OpenAI, OpenRouter…). Keys are encrypted like the collector passwords. Without a row the .env
-- values are used. A prompt with an empty model uses the provider's default model.
CREATE TABLE IF NOT EXISTS ai_provider (
    id          int PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    kind        text NOT NULL DEFAULT 'custom',
    name        text NOT NULL DEFAULT 'Loggi',
    url         text NOT NULL DEFAULT '',
    key_enc     bytea,
    model       text NOT NULL DEFAULT '',
    asr_url     text NOT NULL DEFAULT '',
    asr_key_enc bytea,
    asr_model   text NOT NULL DEFAULT '',
    updated_at  timestamptz NOT NULL DEFAULT now(),
    updated_by  text NOT NULL DEFAULT ''
);
