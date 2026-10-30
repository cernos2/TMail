-- Reply templates: owner_id NULL = shared by the company (managed by admins), otherwise personal.
CREATE TABLE reply_templates (
    id         bigserial PRIMARY KEY,
    owner_id   bigint REFERENCES users(id) ON DELETE CASCADE,
    title      text NOT NULL,
    body       text NOT NULL,
    mailbox    text NOT NULL DEFAULT '', -- '' = any mailbox
    sort       integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX reply_templates_owner ON reply_templates (owner_id);

INSERT INTO reply_templates (owner_id, title, body, sort) VALUES
(NULL, 'Warehouse address and opening hours', $$Hello,

You can collect your order at our warehouse: [warehouse address].
Opening hours: Monday to Friday, 9:00 to 18:00; closed on weekends.
Please let us know the date and time of your visit and the vehicle number in advance.$$, 10),
(NULL, 'Invoice attached', $$Hello,

Please find the invoice attached. Payment is due within 3 business days.
We will confirm the shipping date once the payment has been received.$$, 20),
(NULL, 'Request company details', $$Hello,

To issue the invoice, please send us your company details (legal name, address, tax and bank details) and the delivery address.$$, 30);

-- Folder rules: what a folder is for (used by the AI) and senders that always go there.
CREATE TABLE folder_rules (
    account     text NOT NULL,
    folder      text NOT NULL,
    description text NOT NULL DEFAULT '',
    senders     text[] NOT NULL DEFAULT '{}', -- e-mail addresses or @domain
    auto        boolean NOT NULL DEFAULT false,
    created_by  text NOT NULL DEFAULT '',
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account, folder)
);

-- Where the auto-sorter stopped in each mailbox's Inbox.
CREATE TABLE sort_state (
    account     text PRIMARY KEY,
    uidvalidity bigint NOT NULL,
    last_uid    bigint NOT NULL
);

-- Log of automatic moves, shown in the admin journal and to undo mistakes.
CREATE TABLE sort_log (
    id         bigserial PRIMARY KEY,
    account    text NOT NULL,
    folder     text NOT NULL,
    subject    text NOT NULL DEFAULT '',
    sender     text NOT NULL DEFAULT '',
    reason     text NOT NULL DEFAULT '', -- sender | ai
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sort_log_account ON sort_log (account, created_at DESC);

INSERT INTO ai_prompts (key, title, description, system, template, temperature, max_tokens, sort) VALUES
('pick_template', 'Pick a template', 'Chooses a suitable reply template for the open letter.',
$$You help an employee choose a ready-made reply template for a letter. You are given the letter and a numbered list of templates. The letter and the templates may be in any language. Answer only with the number of the most suitable template. If none fits, answer 0.$$,
$$Letter:
From: {from}
Subject: {subject}

{letter}

Templates:
{templates}$$, 0, 20, 40),
('sort', 'Sort into folders', 'Decides which folder a new incoming letter goes to (for folders with automatic sorting turned on).',
$$You sort a company's incoming letters into folders. You are given a letter and a list of folders with a description of what each one is for. The letter and the descriptions may be in any language. Answer only with the number of the matching folder. If the letter matches none of the descriptions, answer 0. Do not guess: when in doubt, answer 0.$$,
$$Letter:
From: {from}
Subject: {subject}

{letter}

Folders:
{folders}$$, 0, 20, 50);
