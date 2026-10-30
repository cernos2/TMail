-- Counterparties (CRM): companies and people, filled automatically from letters and their attachments.
CREATE TABLE crm_parties (
    id            bigserial PRIMARY KEY,
    kind          text NOT NULL CHECK (kind IN ('company', 'person')),
    name          text NOT NULL,
    inn           text NOT NULL DEFAULT '',
    kpp           text NOT NULL DEFAULT '',
    ogrn          text NOT NULL DEFAULT '',
    legal_address text NOT NULL DEFAULT '',
    bank_name     text NOT NULL DEFAULT '',
    bik           text NOT NULL DEFAULT '',
    account       text NOT NULL DEFAULT '', -- settlement account number
    corr_account  text NOT NULL DEFAULT '',
    website       text NOT NULL DEFAULT '',
    position      text NOT NULL DEFAULT '', -- people: job title
    notes         text NOT NULL DEFAULT '',
    phones        text[] NOT NULL DEFAULT '{}',
    emails        text[] NOT NULL DEFAULT '{}',
    domains       text[] NOT NULL DEFAULT '{}', -- companies: their mail domains
    company_id    bigint REFERENCES crm_parties(id) ON DELETE SET NULL, -- people: where they work
    source        text NOT NULL DEFAULT 'ai',
    search        text NOT NULL DEFAULT '',
    last_letter   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX crm_parties_inn ON crm_parties (inn) WHERE inn <> '';
CREATE INDEX crm_parties_emails ON crm_parties USING gin (emails);
CREATE INDEX crm_parties_domains ON crm_parties USING gin (domains);
CREATE INDEX crm_parties_company ON crm_parties (company_id);

CREATE TABLE crm_addresses (
    id         bigserial PRIMARY KEY,
    party_id   bigint NOT NULL REFERENCES crm_parties(id) ON DELETE CASCADE,
    kind       text NOT NULL DEFAULT 'delivery', -- delivery | post
    address    text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (party_id, address)
);

-- Files from letters, linked to the counterparty (the letter itself stays in the mailbox).
CREATE TABLE crm_files (
    id         bigserial PRIMARY KEY,
    party_id   bigint NOT NULL REFERENCES crm_parties(id) ON DELETE CASCADE,
    account    text NOT NULL,
    folder     text NOT NULL,
    uid        bigint NOT NULL,
    part       text NOT NULL,
    message_id text NOT NULL DEFAULT '',
    name       text NOT NULL,
    mime       text NOT NULL DEFAULT '',
    size       bigint NOT NULL DEFAULT 0,
    kind       text NOT NULL DEFAULT 'other', -- requisites | contract | invoice | other
    sent_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (party_id, account, message_id, part)
);

-- Letters with the counterparty (history in the card).
CREATE TABLE crm_letters (
    party_id   bigint NOT NULL REFERENCES crm_parties(id) ON DELETE CASCADE,
    account    text NOT NULL,
    folder     text NOT NULL,
    uid        bigint NOT NULL,
    message_id text NOT NULL,
    subject    text NOT NULL DEFAULT '',
    sent_at    timestamptz NOT NULL,
    incoming   boolean NOT NULL DEFAULT true,
    PRIMARY KEY (party_id, account, message_id)
);
CREATE INDEX crm_letters_party ON crm_letters (party_id, sent_at DESC);

-- A found value differs from the one in the card: the user confirms or rejects it.
CREATE TABLE crm_suggestions (
    id         bigserial PRIMARY KEY,
    party_id   bigint NOT NULL REFERENCES crm_parties(id) ON DELETE CASCADE,
    field      text NOT NULL,
    value      text NOT NULL,
    source     text NOT NULL DEFAULT '', -- subject of the letter it came from
    status     text NOT NULL DEFAULT 'pending',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (party_id, field, value)
);

-- Letters already analysed (by Message-ID per mailbox) and where the scanner stopped.
CREATE TABLE crm_processed (
    account    text NOT NULL,
    message_id text NOT NULL,
    party_id   bigint,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account, message_id)
);
CREATE TABLE crm_state (
    account     text PRIMARY KEY,
    uidvalidity bigint NOT NULL,
    last_uid    bigint NOT NULL
);

INSERT INTO ai_prompts (key, title, description, system, template, temperature, max_tokens, sort) VALUES
('extract', 'Counterparty cards', 'Takes the sender''s details from a letter and its attachments: company, identifiers, bank details, contact person, phones, delivery addresses.',
$$You extract counterparty details from the business correspondence of our company: the company that owns this mail server. The letter is addressed to our mailbox (its domain is ours), and a service note at the start of the letter names our side. The details of our own company, its employees, addresses and bank details must be IGNORED: we do not need them.
We need the details of the SENDER of the letter and of the sender's organisation: from the signature, the letter text and the attachments (company details sheet, invoice, contract, bank details). Invent nothing: when a value is missing, give an empty string or an empty list.
Answer with JSON ONLY, without explanations, in this form:
{"kind": "company", "sender_company": true, "company": {"name": "", "country": "", "tax_id": "", "other_ids": {}, "kpp": "", "ogrn": "", "legal_address": "", "bank_name": "", "bank_code": "", "account": "", "corr_account": "", "website": "", "phones": []}, "person": {"name": "", "position": "", "phones": []}, "buyer_tax_id": "", "buyer_country": "", "buyer_name": "", "delivery_addresses": [], "attachments": [{"name": "", "type": "other"}]}
kind: company (an organisation or its employee writes), person (a private individual), newsletter (a mailing, advertising, a notification of a service) or none (unclear). The type of an attachment: requisites (company details, bank details sheet), contract, invoice or other.
country: the company's country as an ISO 3166-1 alpha-2 code (US, GB, DE, FR, IN, BR…), judged by its address, identifiers, bank, phone codes or website; empty when unclear.
Identifiers: use the names and formats of the company's own country. tax_id is its main tax or registration number there (for example EIN in the USA, the company number or VAT number in Europe, GSTIN in India, CNPJ in Brazil, INN in Russia), the value only, without its label. other_ids holds its other identifiers as {"label as written in the document": "value"}, for example {"VAT": "DE123456789", "Company number": "01234567"}. kpp, ogrn and corr_account (KPP, OGRN, correspondent account) exist only for Russian companies: leave them empty for other countries.
bank_code: the code of the company's bank (BIC/SWIFT, routing number, sort code, IFSC, BIK…); account: the account number or IBAN.
In an invoice there are two companies: the supplier (who receives the payment) and the buyer (who pays); the bank details in an invoice are always the supplier's. buyer_tax_id, buyer_country and buyer_name are the buyer's, when there is an invoice.
Write the company name as in its documents, with its legal form (Example Ltd, Muster GmbH, Exemplo Ltda.). Phones in the international format, with the country code (+44 20 7946 0000). The person's full name as in the signature.$$,
$$Our mailbox: {mailbox}
From: {from}
Subject: {subject}

Letter text:
{letter}

Attachments:
{attachments}$$, 0.1, 1500, 60);
