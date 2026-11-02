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

# Relay (sending through an external SMTP server), set by an administrator in the webmail:
# TMAIL writes /etc/postfix/dyn/relay.conf (+ relay.secret), this applies it and reports back.
DYN=/etc/postfix/dyn
apply_relay() {
  local host="" port="" wrapper="no"
  if [ -s "$DYN/relay.conf" ]; then
    host=$(sed -n 's/^host=//p' "$DYN/relay.conf" | head -1)
    port=$(sed -n 's/^port=//p' "$DYN/relay.conf" | head -1)
    wrapper=$(sed -n 's/^wrapper=//p' "$DYN/relay.conf" | head -1)
    case "$host" in ''|*[!a-z0-9.-]*) host="";; esac
    case "$port" in ''|*[!0-9]*) host="";; esac
    [ "$wrapper" = yes ] || wrapper=no
  fi
  if [ -n "$host" ]; then
    if [ -s "$DYN/relay.secret" ]; then
      install -m 600 -o root -g root "$DYN/relay.secret" /etc/postfix/sasl_passwd
      postconf -e smtp_sasl_auth_enable=yes smtp_sasl_password_maps=texthash:/etc/postfix/sasl_passwd \
        smtp_sasl_security_options=noanonymous smtp_sasl_tls_security_options=noanonymous
    else
      rm -f /etc/postfix/sasl_passwd
      postconf -e smtp_sasl_auth_enable=no
    fi
    postconf -e "relayhost=[$host]:$port" smtp_tls_security_level=encrypt "smtp_tls_wrappermode=$wrapper"
    echo "on $host:$port $(date -u +%FT%TZ)" > "$DYN/relay.applied" 2>/dev/null || true
  else
    rm -f /etc/postfix/sasl_passwd
    postconf -e relayhost= smtp_sasl_auth_enable=no smtp_tls_wrappermode=no smtp_tls_security_level=dane
    echo "off $(date -u +%FT%TZ)" > "$DYN/relay.applied" 2>/dev/null || true
  fi
}
relay_sum() { cat "$DYN/relay.conf" "$DYN/relay.secret" 2>/dev/null | sha256sum | cut -d' ' -f1; }
mkdir -p "$DYN"
apply_relay

# MTA-STS resolver for outgoing mail (smtp_tls_policy_maps), as an unprivileged user, restarted
# if it ever stops (without it outgoing mail would wait in the queue).
(
  while true; do
    HOME=/nonexistent setpriv --reuid=nobody --regid=nogroup --clear-groups mta-sts-daemon -c /etc/mta-sts-daemon.yml
    echo "mta-sts-daemon stopped, restarting in 5 s"
    sleep 5
  done
) &
for _ in $(seq 1 20); do (exec 3<>/dev/tcp/127.0.0.1/8461) 2>/dev/null && break; sleep 0.5; done

postfix set-permissions >/dev/null 2>&1 || true
postfix check

# Reload when the certificate is renewed or the relay settings change.
(
  last=$(sha256sum "$TLS" | cut -d' ' -f1)
  rlast=$(relay_sum)
  n=0
  while sleep 15; do
    rcur=$(relay_sum)
    if [ "$rcur" != "$rlast" ]; then echo "relay settings changed, reloading"; apply_relay; postfix reload; rlast=$rcur; fi
    n=$((n + 1)); [ $((n % 4)) -eq 0 ] || continue
    cur=$(sha256sum "$TLS" | cut -d' ' -f1)
    if [ "$cur" != "$last" ]; then echo "certificate changed, reloading"; postfix reload; last=$cur; fi
  done
) &

exec postfix start-fg
