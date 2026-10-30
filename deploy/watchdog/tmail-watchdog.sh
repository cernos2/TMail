#!/bin/bash
# TMAIL watchdog (systemd timer, every 5 minutes): real checks of every part of the service, and
# a restart of the part that fails, so a failure is repaired before anyone notices it.
# Every repair is logged to /var/log/tmail-watchdog.log and to the admin audit log.
set -u
cd "${TMAIL_DIR:-/opt/zmail}" || exit 1
HOST=$(sed -n 's/^MAIL_HOSTNAME=//p' .env | head -1)
LOG=/var/log/tmail-watchdog.log
STATE=/var/lib/tmail-watchdog; mkdir -p "$STATE"
log() { echo "$(date '+%F %T') $*" >>"$LOG"; }
sql() { docker compose exec -T postgres psql -U zmail -d zmail -Atqc "$1" 2>/dev/null; }
audit() { # action, details
  local d=${2//\'/\'\'}
  sql "INSERT INTO audit_log (actor_login, action, target, details) VALUES ('watchdog', 'watchdog.$1', '$1', jsonb_build_object('details', '$d'))" >/dev/null
}
# A part is restarted only after two failed checks in a row (a slow moment is not a failure),
# and not more often than once in 15 minutes.
repair() { # service, reason[, check name when not the service's own]
  local svc=$1 why=$2 f="$STATE/${3:-$1}.fail"
  local n=$(( $(cat "$f" 2>/dev/null || echo 0) + 1 )); echo $n >"$f"
  log "FAIL $svc: $why (check $n)"
  [ $n -lt 2 ] && return
  local last=$(stat -c %Y "$STATE/$svc.restart" 2>/dev/null || echo 0)
  if [ $(( $(date +%s) - last )) -lt 900 ]; then log "  $svc restarted less than 15 min ago — waiting"; return; fi
  log "RESTART $svc"
  docker compose restart "$svc" >>"$LOG" 2>&1 || docker compose up -d "$svc" >>"$LOG" 2>&1
  touch "$STATE/$svc.restart"; rm -f "$f" "$STATE/collector.sig"
  audit "$svc" "restarted: $why"
}
ok() { rm -f "$STATE/$1.fail"; }

# 1. Every container runs.
for svc in postgres redis unbound rspamd certs postfix dovecot web preview garage clamav; do
  st=$(docker compose ps -a --format '{{.State}}' "$svc" 2>/dev/null)
  if [ "$st" != running ]; then
    log "DOWN $svc ($st) — starting"
    docker compose up -d "$svc" >>"$LOG" 2>&1
    audit "$svc" "was $st, started"
  fi
done

# 2. Processes pile-up (unreaped processes end in "Cannot fork"): zombies per container.
for svc in web preview dovecot postfix clamav rspamd; do
  z=$(docker compose top "$svc" 2>/dev/null | grep -c '<defunct>')
  if [ "${z:-0}" -gt 50 ]; then repair "$svc" "$z zombie processes" "$svc-zombies"; else ok "$svc-zombies"; fi
done

# 3. The site answers.
code=$(curl -s -o /dev/null -m 20 -w '%{http_code}' --resolve "$HOST:443:127.0.0.1" "https://$HOST/")
if [ "$code" = 200 ]; then ok web; else repair web "site answered $code"; fi

# 4. The mail server answers: IMAP banner on 993, SMTP on 25.
if timeout 15 openssl s_client -quiet -connect 127.0.0.1:993 </dev/null 2>/dev/null | head -1 | grep -q '^\* OK'; then ok dovecot; else repair dovecot "IMAP 993 does not answer"; fi
if timeout 15 bash -c 'exec 3<>/dev/tcp/127.0.0.1/25; head -1 <&3' 2>/dev/null | grep -q '^220'; then ok postfix; else repair postfix "SMTP 25 does not answer"; fi

# 5. Document previews: the service's own self-test of LibreOffice (/health is 503 when it fails).
SOCK=/var/lib/docker/volumes/zmail_previewsock/_data/preview.sock
res=$(curl -s -m 20 --unix-socket "$SOCK" -w ' %{http_code}' http://preview/health)
if [ "${res##* }" = 200 ]; then ok preview; else repair preview "health: $res"; fi

# 6. Antivirus answers (files can't be downloaded while it is down).
if docker compose exec -T clamav sh -c 'echo PING | nc -w 10 127.0.0.1 3310' 2>/dev/null | grep -q PONG; then ok clamav; else repair clamav "clamd does not answer PING"; fi

# 7. Mail collectors are not stuck: the accounts are checked every 1–2 minutes, and a long import
#    counts its letters after every batch — nothing of this moving for 30 minutes means a hang.
sig=$(sql "SELECT max(last_run)::text || ' ' || sum(imported) FROM external_accounts WHERE enabled")
if [ -n "$sig" ]; then
  if [ "$sig" != "$(cat "$STATE/collector.sig" 2>/dev/null)" ]; then echo "$sig" >"$STATE/collector.sig"; ok collector
  elif [ $(( $(date +%s) - $(stat -c %Y "$STATE/collector.sig") )) -ge 1800 ]; then
    repair web "collectors stuck: no check and no letter for 30 min" collector
  fi
fi

fails=$(ls "$STATE"/*.fail 2>/dev/null | wc -l)
[ "$fails" = 0 ] && log "ok" || log "checked: $fails failing"

# Keep the log small.
[ $(stat -c %s "$LOG" 2>/dev/null || echo 0) -gt 5000000 ] && tail -c 2000000 "$LOG" >"$LOG.tmp" && mv "$LOG.tmp" "$LOG"
exit 0
