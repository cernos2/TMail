#!/bin/bash
set -euo pipefail
: "${MAIL_HOSTNAME:?}" "${MTA_DB_PASSWORD:?}"

# Render database lookup maps (password only lives in the container).
for f in /etc/postfix/pgsql/*.cf.tmpl; do
  envsubst '$MTA_DB_PASSWORD' < "$f" > "${f%.tmpl}"
done
chown root:postfix /etc/postfix/pgsql/*.cf
chmod 640 /etc/postfix/pgsql/*.cf

postconf -e "myhostname=${MAIL_HOSTNAME}"

TLS=/etc/zmail/tls/fullchain.pem
for _ in $(seq 1 90); do [ -s "$TLS" ] && break; echo "waiting for TLS certificate..."; sleep 2; done
[ -s "$TLS" ] || { echo "no TLS certificate in /etc/zmail/tls" >&2; exit 1; }

postfix set-permissions >/dev/null 2>&1 || true
postfix check

# Reload when the certificate is renewed.
(
  last=$(sha256sum "$TLS" | cut -d' ' -f1)
  while sleep 60; do
    cur=$(sha256sum "$TLS" | cut -d' ' -f1)
    if [ "$cur" != "$last" ]; then echo "certificate changed, reloading"; postfix reload; last=$cur; fi
  done
) &

exec postfix start-fg
