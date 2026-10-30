-- "Our side": companies, INNs, mail domains and addresses that are never counterparties
-- (besides the TMAIL mailboxes and users, which are always ours).
CREATE TABLE IF NOT EXISTS crm_self (
    kind       text NOT NULL,        -- name | inn | domain | email | signature (a name of our side, no card)
    value      text NOT NULL,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, value)
);
