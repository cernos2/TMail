#!/bin/bash
set -euo pipefail
: "${MAIL_DOMAIN:?}" "${MTA_DB_PASSWORD:?}" "${DOVECOT_MASTER_PASSWORD:?}" "${DOVEADM_API_KEY:?}"

envsubst '$MAIL_DOMAIN $DOVEADM_API_KEY' < /etc/dovecot/dovecot.conf.tmpl > /etc/dovecot/dovecot.conf
chmod 640 /etc/dovecot/dovecot.conf && chown root:dovecot /etc/dovecot/dovecot.conf
envsubst '$MTA_DB_PASSWORD' < /etc/dovecot/dovecot-sql.conf.ext.tmpl > /etc/dovecot/dovecot-sql.conf.ext
chmod 600 /etc/dovecot/dovecot-sql.conf.ext

TLS=/etc/zmail/tls/fullchain.pem
for _ in $(seq 1 90); do [ -s "$TLS" ] && break; echo "waiting for TLS certificate..."; sleep 2; done
[ -s "$TLS" ] || { echo "no TLS certificate in /etc/zmail/tls" >&2; exit 1; }

# The master login (used by the webmail to open any mailbox) only works from this host.
HASH=$(doveadm pw -s BLF-CRYPT -p "$DOVECOT_MASTER_PASSWORD")
echo "zmail-app:$HASH::::::allow_nets=127.0.0.1/32,::1/128" > /etc/dovecot/master-users
# The same login for searches by sender or recipient: without the full-text index, which needs ~1 ms
# per hit (65 000 letters from one marketplace address = a minute), while the header cache takes 2 s.
echo "zmail-search:$HASH::::::allow_nets=127.0.0.1/32,::1/128 userdb_mail_plugins=quota" >> /etc/dovecot/master-users
chown root:dovecot /etc/dovecot/master-users
chmod 640 /etc/dovecot/master-users

sievec /etc/dovecot/sieve/before.d
chown vmail:vmail /var/vmail

(
  last=$(sha256sum "$TLS" | cut -d' ' -f1)
  while sleep 60; do
    cur=$(sha256sum "$TLS" | cut -d' ' -f1)
    if [ "$cur" != "$last" ]; then echo "certificate changed, reloading"; doveadm reload; last=$cur; fi
  done
) &

exec dovecot -F
