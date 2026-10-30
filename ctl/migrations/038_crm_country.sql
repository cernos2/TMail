-- Counterparty cards for companies of any country.
-- inn now holds the main tax / registration ID of a company in its country (INN in Russia, EIN in the
-- USA, a VAT or company number elsewhere); bik the bank code (BIC/SWIFT, routing number, sort code,
-- BIK); account the account number or IBAN. kpp, ogrn and corr_account stay for Russian companies.
ALTER TABLE crm_parties ADD COLUMN IF NOT EXISTS country text NOT NULL DEFAULT ''; -- ISO 3166-1 alpha-2, '' when not known
ALTER TABLE crm_parties ADD COLUMN IF NOT EXISTS ids jsonb NOT NULL DEFAULT '{}'; -- other identifiers: {"VAT": "DE123456789"}

-- The cards made so far with a valid Russian INN (10 or 12 digits, checksum) are Russian companies.
UPDATE crm_parties p SET country = 'RU'
FROM (SELECT id, string_to_array(regexp_replace(inn, '[^0-9]', '', 'g'), NULL)::int[] AS d
      FROM crm_parties WHERE kind = 'company' AND country = '' AND inn ~ '^([0-9]{10}|[0-9]{12})$') x
WHERE p.id = x.id AND CASE cardinality(x.d)
    WHEN 10 THEN (2*d[1] + 4*d[2] + 10*d[3] + 3*d[4] + 5*d[5] + 9*d[6] + 4*d[7] + 6*d[8] + 8*d[9]) % 11 % 10 = d[10]
    WHEN 12 THEN (7*d[1] + 2*d[2] + 4*d[3] + 10*d[4] + 3*d[5] + 5*d[6] + 9*d[7] + 4*d[8] + 6*d[9] + 8*d[10]) % 11 % 10 = d[11]
             AND (3*d[1] + 7*d[2] + 2*d[3] + 4*d[4] + 10*d[5] + 3*d[6] + 5*d[7] + 9*d[8] + 4*d[9] + 6*d[10] + 8*d[11]) % 11 % 10 = d[12]
    ELSE false END;
