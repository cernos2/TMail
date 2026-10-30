#!/bin/sh
# Obtains and renews the Let's Encrypt certificate for MAIL_HOSTNAME (HTTP-01 via webroot /acme,
# served on port 80 by the web service)
# and publishes it to /tls/{fullchain,privkey}.pem for Postfix and Dovecot.
# Until a real certificate is issued, a temporary self-signed one keeps the services running.
set -u
H="${MAIL_HOSTNAME:?}"
# Extra names on the same certificate (e.g. files.<domain> for large-file links), space separated.
EXTRA="${EXTRA_DOMAINS:-}"
LIVE="/etc/letsencrypt/live/$H"

publish() {
  [ -s "$LIVE/fullchain.pem" ] || return 1
  if ! cmp -s "$LIVE/fullchain.pem" /tls/fullchain.pem; then
    cp -L "$LIVE/privkey.pem" /tls/privkey.pem.new && chmod 600 /tls/privkey.pem.new
    cp -L "$LIVE/fullchain.pem" /tls/fullchain.pem.new && chmod 644 /tls/fullchain.pem.new
    mv /tls/privkey.pem.new /tls/privkey.pem
    mv /tls/fullchain.pem.new /tls/fullchain.pem
    echo "published certificate: $(openssl x509 -in /tls/fullchain.pem -noout -subject -enddate | tr '\n' ' ')"
  fi
}

obtain() {
  set -- -d "$H"
  for d in $EXTRA; do set -- "$@" -d "$d"; done
  certbot certonly --webroot -w /acme --non-interactive --agree-tos --register-unsafely-without-email --expand "$@"
}

# True when the current certificate lacks one of the extra names (they were added later).
missing_names() {
  for d in $EXTRA; do
    openssl x509 -in "$LIVE/fullchain.pem" -noout -ext subjectAltName 2>/dev/null | grep -q "DNS:$d" || return 0
  done
  return 1
}

if [ ! -s /tls/fullchain.pem ]; then
  echo "creating temporary self-signed certificate"
  openssl req -x509 -newkey rsa:2048 -nodes -days 30 -subj "/CN=$H" \
    -keyout /tls/privkey.pem -out /tls/fullchain.pem 2>/dev/null
  chmod 600 /tls/privkey.pem
fi

while true; do
  if [ -s "$LIVE/fullchain.pem" ] && missing_names; then
    obtain && publish || echo "could not add $EXTRA to the certificate yet (DNS?), retrying later"
  fi
  if [ -s "$LIVE/fullchain.pem" ]; then
    certbot renew --webroot -w /acme --non-interactive --quiet || echo "renewal attempt failed, will retry"
    publish
    sleep 43200
  else
    obtain && publish || echo "could not obtain certificate yet (DNS/port 80?), retrying in 1h"
    [ -s "$LIVE/fullchain.pem" ] && continue
    sleep 3600
  fi
done
