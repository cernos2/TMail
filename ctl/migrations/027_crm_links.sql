-- Companies stay separate cards; people and companies are tied by links instead of being merged.

-- Our own company (marked "It's us"): a card like any other, never a counterparty.
ALTER TABLE crm_parties ADD COLUMN IF NOT EXISTS own boolean NOT NULL DEFAULT false;

-- One address may write for several companies: which ones and how often.
-- crm_parties.company_id stays as the main one (the most letters).
CREATE TABLE IF NOT EXISTS crm_person_companies (
    person_id   bigint NOT NULL REFERENCES crm_parties(id) ON DELETE CASCADE,
    company_id  bigint NOT NULL REFERENCES crm_parties(id) ON DELETE CASCADE,
    letters     int NOT NULL DEFAULT 0,
    last_letter timestamptz,
    PRIMARY KEY (person_id, company_id)
);
CREATE INDEX IF NOT EXISTS crm_person_companies_company ON crm_person_companies (company_id);

INSERT INTO crm_person_companies (person_id, company_id, letters, last_letter)
SELECT p.id, p.company_id,
       (SELECT count(*) FROM crm_letters a JOIN crm_letters b ON a.account = b.account AND a.message_id = b.message_id
         WHERE a.party_id = p.id AND b.party_id = p.company_id),
       p.last_letter
FROM crm_parties p WHERE p.kind = 'person' AND p.company_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Company ↔ company, from documents: supplier (from supplies to, by an invoice) or
-- via (documents of "from" came through "to", e.g. a forwarded supplier invoice).
CREATE TABLE IF NOT EXISTS crm_company_links (
    from_id     bigint NOT NULL REFERENCES crm_parties(id) ON DELETE CASCADE,
    to_id       bigint NOT NULL REFERENCES crm_parties(id) ON DELETE CASCADE,
    role        text NOT NULL,
    letters     int NOT NULL DEFAULT 0,
    last_letter timestamptz,
    PRIMARY KEY (from_id, to_id, role),
    CHECK (from_id <> to_id)
);
CREATE INDEX IF NOT EXISTS crm_company_links_to ON crm_company_links (to_id);

-- Name variants confirmed by merging cards: the key of the old name points at the card it went to.
CREATE TABLE IF NOT EXISTS crm_aliases (
    key      text PRIMARY KEY,
    party_id bigint NOT NULL REFERENCES crm_parties(id) ON DELETE CASCADE
);
