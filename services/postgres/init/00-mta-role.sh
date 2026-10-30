#!/bin/sh
# Runs once, on first database initialisation.
# mta_ro: read-only role for Postfix and Dovecot; it only sees views in schema "mta".
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<EOSQL
CREATE ROLE mta_ro LOGIN PASSWORD '${MTA_DB_PASSWORD}';
CREATE SCHEMA IF NOT EXISTS mta AUTHORIZATION ${POSTGRES_USER};
GRANT USAGE ON SCHEMA mta TO mta_ro;
ALTER DEFAULT PRIVILEGES FOR ROLE ${POSTGRES_USER} IN SCHEMA mta GRANT SELECT ON TABLES TO mta_ro;
EOSQL
