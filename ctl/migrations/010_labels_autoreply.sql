-- Labels (as in Gmail): shared list of names and colours; a letter carries a label as the IMAP
-- keyword "$tl_<id>", so labels live in the mailbox itself and survive any client.
CREATE TABLE labels (
    id         bigserial PRIMARY KEY,
    name       text NOT NULL UNIQUE,
    color      text NOT NULL DEFAULT 'blue',
    sort       integer NOT NULL DEFAULT 0,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Auto-replies per mailbox ("on vacation"): the text is written by Loggi from the chosen reason and
-- edited by the user; delivery uses a Sieve vacation script installed for the mailbox.
CREATE TABLE autoreplies (
    mailbox     text PRIMARY KEY,
    enabled     boolean NOT NULL DEFAULT false,
    reason      text NOT NULL DEFAULT 'vacation',
    date_from   date,
    date_to     date,
    substitute  text NOT NULL DEFAULT '', -- who to contact meanwhile
    notes       text NOT NULL DEFAULT '',
    subject     text NOT NULL DEFAULT '',
    body        text NOT NULL DEFAULT '',
    updated_by  text NOT NULL DEFAULT '',
    updated_at  timestamptz NOT NULL DEFAULT now()
);

INSERT INTO ai_prompts (key, title, description, system, template, temperature, max_tokens, sort) VALUES
('autoreply', 'Auto-reply text', 'Writes the auto-reply text from the reason (vacation, business trip…) and the dates chosen by the employee.',
$$You write a short, polite auto-reply for a company's business mailbox. 3–5 sentences: a greeting, why there is no reply now and until when, whom to contact about urgent matters (if a substitute is given), and thanks. Write in the language of the additional notes if they contain text; otherwise in the language of the absence reason. Do not invent anything: use only the data from the request. Do not write a subject line or a signature. Return only the text of the letter.$$,
$$Reason for absence: {instructions}
Employee: {user}
Mailbox: {mailbox}
Additional notes: {draft}$$, 0.4, 600, 70);
