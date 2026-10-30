#!/bin/bash
# TMAIL installer: a complete mail server with webmail on one Linux server, configured end to end.
#
#   git clone <repository> /opt/tmail && cd /opt/tmail && sudo ./install.sh
#
# What it does: checks the server (memory, disk, ports, outgoing port 25), installs Docker, asks a few
# questions (domain, administrator, language, country, time zone, AI, backups), generates all secrets,
# builds and starts every service — Postfix, Dovecot, Rspamd (spam), ClamAV (antivirus), PostgreSQL,
# Redis, Garage (file storage), the webmail —, creates the domain with its DKIM key, the first mailbox
# and the administrator, sets up the firewall, fail2ban, the watchdog and nightly backups, and prints
# the DNS records to add at the domain registrar.
# Run it again to repair or update an installation: existing settings and data are kept.
#
# Unattended: every question has an option, e.g.
#   ./install.sh --yes --domain example.com --admin-login admin --lang en --country US \
#                --timezone America/New_York --ai openrouter --ai-key sk-or-… [--ai-model openai/gpt-4o-mini]
set -euo pipefail
cd "$(dirname "$(readlink -f "$0")")"
DIR=$(pwd)

# ---------- output ----------
if [ -t 1 ]; then B=$'\e[1m'; G=$'\e[32m'; Y=$'\e[33m'; R=$'\e[31m'; N=$'\e[0m'; else B= G= Y= R= N=; fi
step() { echo; echo "${B}==> $*${N}"; }
ok()   { echo "  ${G}✓${N} $*"; }
warn() { echo "  ${Y}!${N} $*"; }
die()  { echo "${R}✗ $*${N}" >&2; exit 1; }

# ---------- options ----------
YES=0 FORCE=0
ORG_= DOMAIN= HOSTNAME_= FILES_HOST= ADMIN_LOGIN= ADMIN_NAME= ADMIN_MAILBOX= LANG_= COUNTRY_= TZ_=
AI= AI_URL= AI_KEY= AI_MODEL_= BACKUP_REPO=
while [ $# -gt 0 ]; do
  case "$1" in
    --yes|-y) YES=1 ;;
    --force) FORCE=1 ;;
    --domain) DOMAIN=$2; shift ;;
    --hostname) HOSTNAME_=$2; shift ;;
    --files-hostname) FILES_HOST=$2; shift ;;
    --admin-login) ADMIN_LOGIN=$2; shift ;;
    --admin-name) ADMIN_NAME=$2; shift ;;
    --admin-mailbox) ADMIN_MAILBOX=$2; shift ;;
    --lang) LANG_=$2; shift ;;
    --country) COUNTRY_=$2; shift ;;
    --org) ORG_=$2; shift ;;
    --timezone) TZ_=$2; shift ;;
    --ai) AI=$2; shift ;;
    --ai-url) AI_URL=$2; shift ;;
    --ai-key) AI_KEY=$2; shift ;;
    --ai-model) AI_MODEL_=$2; shift ;;
    --backup-repository) BACKUP_REPO=$2; shift ;;
    -h|--help) sed -n '2,17p' "$0"; exit 0 ;;
    *) die "unknown option $1 (see --help)" ;;
  esac
  shift
done

ask() { # variable, question, default
  local var=$1 q=$2 def=${3:-} ans
  if [ -n "${!var}" ]; then return; fi
  if [ "$YES" = 1 ] || [ ! -t 0 ]; then printf -v "$var" '%s' "$def"; return; fi
  read -r -p "  $q${def:+ [$def]}: " ans
  printf -v "$var" '%s' "${ans:-$def}"
}
envget() { [ -f .env ] && sed -n "s/^$1=//p" .env | tail -1 || true; }
secret() { openssl rand -hex "${1:-24}"; }

[ "$(id -u)" = 0 ] || die "run as root: sudo ./install.sh"

# ---------- 1. checks ----------
step "Checking the server"
. /etc/os-release 2>/dev/null || true
ok "system: ${PRETTY_NAME:-unknown}"
MEM_MB=$(awk '/MemTotal/{print int($2/1024)}' /proc/meminfo)
SWAP_MB=$(awk '/SwapTotal/{print int($2/1024)}' /proc/meminfo)
if [ "$MEM_MB" -lt 1800 ]; then
  [ "$FORCE" = 1 ] || die "${MEM_MB} MB of memory: at least 2 GB is needed (4 GB recommended). --force to try anyway"
  warn "${MEM_MB} MB of memory — expect slowness"
elif [ "$MEM_MB" -lt 3800 ]; then warn "${MEM_MB} MB of memory: fine for a small team; 4 GB is recommended for more users and AI features"
else ok "memory: ${MEM_MB} MB"; fi
# Below 4 GB the rarely used part of the antivirus database lives in swap: make sure there is some.
if [ "$MEM_MB" -lt 3800 ] && [ "$SWAP_MB" -lt 1500 ] && [ ! -e /swapfile ]; then
  fallocate -l 3G /swapfile 2>/dev/null || dd if=/dev/zero of=/swapfile bs=1M count=3072 status=none
  chmod 600 /swapfile && mkswap /swapfile >/dev/null && swapon /swapfile
  grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
  ok "3 GB swap file created"
fi
DISK_GB=$(df -BG --output=avail / | tail -1 | tr -dc 0-9)
[ "$DISK_GB" -ge 15 ] || [ "$FORCE" = 1 ] || die "only ${DISK_GB} GB free on /: at least 20 GB is needed"
ok "free disk: ${DISK_GB} GB"
for c in curl openssl; do command -v $c >/dev/null || NEED_PKGS=1; done
if [ "${NEED_PKGS:-0}" = 1 ] || ! command -v python3 >/dev/null; then
  if command -v apt-get >/dev/null; then apt-get update -qq && apt-get install -y -qq curl openssl ca-certificates python3 >/dev/null
  elif command -v dnf >/dev/null; then dnf install -y -q curl openssl ca-certificates python3
  else die "install curl, openssl and python3 first"; fi
fi
PUBLIC_IP=$(curl -4 -fsS -m 10 https://api.ipify.org 2>/dev/null || curl -4 -fsS -m 10 https://ifconfig.me 2>/dev/null || ip -4 route get 1.1.1.1 | awk '{for(i=1;i<NF;i++) if($i=="src") print $(i+1)}')
[ -n "$PUBLIC_IP" ] || die "cannot find the public IPv4 address of this server"
ok "public IP: $PUBLIC_IP"
if timeout 8 bash -c '</dev/tcp/gmail-smtp-in.l.google.com/25' 2>/dev/null; then ok "outgoing port 25 is open"
else warn "outgoing port 25 seems blocked: the server will receive mail but cannot deliver it. Ask the hosting provider to open port 25."; fi
busy=""
for p in 25 80 443 465 587 143 993 995; do
  if ss -Hltn "sport = :$p" 2>/dev/null | grep -q . && ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q .; then busy="$busy $p"; fi
done
[ -z "$busy" ] || [ "$FORCE" = 1 ] || die "ports already in use:$busy — stop the other mail/web server first (or --force)"

# ---------- 2. questions ----------
step "Settings (Enter keeps the suggestion)"
[ -f .env ] && ok "existing .env found: its values are kept"
SYS_TZ=$(timedatectl show -p Timezone --value 2>/dev/null || cat /etc/timezone 2>/dev/null || echo UTC)
ask DOMAIN "Mail domain (addresses will be name@domain)" "$(envget MAIL_DOMAIN)"
[ -n "$DOMAIN" ] || die "the mail domain is required (--domain example.com)"
ask HOSTNAME_ "Server name for mail and webmail" "$(envget MAIL_HOSTNAME || true)"; HOSTNAME_=${HOSTNAME_:-mail.$DOMAIN}
ask FILES_HOST "Name for links to large files" "$(envget FILES_HOSTNAME)"; FILES_HOST=${FILES_HOST:-files.$DOMAIN}
ask LANG_ "Interface language (en, ru, de, es, fr, zh-CN, ar, …)" "$(envget DEFAULT_LANG)"; LANG_=${LANG_:-en}
ask COUNTRY_ "Country (two letters: US, GB, DE, RU, IN, …) — company details are recognised for it" "$(envget COUNTRY)"
ask ORG_ "Organisation name (shown to recipients on file download pages)" "$(envget ORG_NAME)"; ORG_=${ORG_:-$DOMAIN}
ask TZ_ "Time zone" "$(envget DEFAULT_TIMEZONE)"; TZ_=${TZ_:-$SYS_TZ}
[ -e "/usr/share/zoneinfo/$TZ_" ] || die "unknown time zone $TZ_ (e.g. Europe/Berlin, America/New_York)"
FIRST_RUN=0; grep -q '^DB_PASSWORD=.' .env 2>/dev/null || FIRST_RUN=1
if [ "$FIRST_RUN" = 1 ]; then
  ask ADMIN_LOGIN "Administrator login for the webmail" "admin"
  ask ADMIN_NAME "Administrator's name" "Administrator"
  ask ADMIN_MAILBOX "Administrator's mailbox" "postmaster@$DOMAIN"
fi

echo "  AI assistant (replies, summaries, sorting, contact cards). Any OpenAI-compatible API:"
echo "    none | openai | openrouter | custom (your own model: vLLM, Ollama, LM Studio…)"
ask AI "AI" "$( [ -n "$(envget AI_API_URL)$(envget LOGGI_API_URL)" ] && echo keep || echo none)"
case "$AI" in
  keep|none) ;;
  openai) AI_URL=${AI_URL:-https://api.openai.com}; ask AI_KEY "OpenAI API key"; ask AI_MODEL_ "Model" "gpt-4o-mini" ;;
  openrouter) AI_URL=${AI_URL:-https://openrouter.ai/api}; ask AI_KEY "OpenRouter API key (openrouter.ai/keys)"; ask AI_MODEL_ "Model" "openai/gpt-4o-mini" ;;
  custom) ask AI_URL "API address (e.g. http://10.0.0.5:8000)"; ask AI_KEY "API key (empty if none)"; ask AI_MODEL_ "Model name" ;;
  *) die "--ai must be none, openai, openrouter or custom" ;;
esac
echo "  Backups: a folder on this server, or (better) storage elsewhere: s3:https://host/bucket, sftp:user@host:/path"
ask BACKUP_REPO "Backup repository" "$(envget BACKUP_REPOSITORY)"; BACKUP_REPO=${BACKUP_REPO:-/var/backups/tmail/restic}

# ---------- 3. Docker and host tools ----------
step "Installing Docker and system tools"
if ! command -v docker >/dev/null || ! docker compose version >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sh >/dev/null
  systemctl enable --now docker >/dev/null 2>&1 || true
fi
ok "$(docker --version)"
if command -v apt-get >/dev/null; then
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nftables fail2ban >/dev/null || warn "nftables/fail2ban not installed"
fi

# ---------- 4. configuration ----------
step "Writing the configuration (.env)"
touch .env && chmod 600 .env
setenv() { # key value — replaces or appends, never prints the value
  local k=$1 v=$2
  if grep -q "^$k=" .env; then python3 - "$k" "$v" <<'PY'
import sys, re
k, v = sys.argv[1], sys.argv[2]
p = '.env'; s = open(p).read()
s = re.sub(rf'^{re.escape(k)}=.*$', lambda m: f'{k}={v}', s, flags=re.M)
open(p, 'w').write(s)
PY
  else printf '%s=%s\n' "$k" "$v" >> .env; fi
}
keep() { [ -n "$(envget "$1")" ] || setenv "$1" "$2"; }
setenv MAIL_DOMAIN "$DOMAIN"
setenv MAIL_HOSTNAME "$HOSTNAME_"
setenv FILES_HOSTNAME "$FILES_HOST"
setenv PUBLIC_IP "$PUBLIC_IP"
setenv DEFAULT_LANG "$LANG_"
setenv DEFAULT_TIMEZONE "$TZ_"
setenv COUNTRY "$(echo "$COUNTRY_" | tr a-z A-Z)"
setenv ORG_NAME "$ORG_"
keep DKIM_SELECTOR "s$(date +%Y)"
keep DB_PASSWORD "$(secret)"
keep MTA_DB_PASSWORD "$(secret)"
keep DOVECOT_MASTER_PASSWORD "$(secret)"
keep DOVEADM_API_KEY "$(secret)"
keep GARAGE_RPC_SECRET "$(secret 32)"
keep BACKUP_PASSWORD "$(secret)"
setenv BACKUP_REPOSITORY "$BACKUP_REPO"
if [ "$AI" != keep ] && [ "$AI" != none ]; then
  setenv AI_API_URL "$AI_URL"; setenv AI_API_KEY "$AI_KEY"; setenv AI_MODEL "$AI_MODEL_"
fi
ok ".env ready (secrets generated, file readable by root only)"

# ---------- 5. services ----------
step "Building the services (the first time takes 10–20 minutes)"
docker compose build --pull -q
docker compose up -d postgres redis
for i in $(seq 1 60); do docker compose exec -T postgres pg_isready -U zmail >/dev/null 2>&1 && break; sleep 2; done
docker compose run --rm ctl migrate
docker compose up -d
ok "services started"
deploy/garage-init.sh >/dev/null && ok "file storage ready"
docker compose up -d web >/dev/null  # picks up the storage keys written by garage-init

step "Building the web interface"
deploy/build-web.sh

# ---------- 6. domain, DKIM, administrator ----------
step "Domain, DKIM key and administrator"
Z() { docker compose run --rm -T ctl "$@"; }
Z domain list 2>/dev/null | grep -qw "$DOMAIN" || Z domain add "$DOMAIN"
Z dkim gen "$DOMAIN" >/dev/null 2>&1 || true
ADMIN_PASS=
if [ "$FIRST_RUN" = 1 ]; then
  ADMIN_PASS=$(openssl rand -base64 18 | tr -d '/+=' | cut -c1-16)
  MAILBOX_PASS=$(openssl rand -base64 18 | tr -d '/+=' | cut -c1-16)
  Z mailbox add "$ADMIN_MAILBOX" --name "$ADMIN_NAME" --password "$MAILBOX_PASS" >/dev/null
  Z user add "$ADMIN_LOGIN" --name "$ADMIN_NAME" --password "$ADMIN_PASS" --admin yes --mailboxes "$ADMIN_MAILBOX" >/dev/null
  Z access grant "$ADMIN_LOGIN" "$ADMIN_MAILBOX" --personal yes >/dev/null || true
  # postmaster@ and abuse@ are required by the mail standards: deliver them to the administrator.
  for a in postmaster abuse; do
    [ "$a@$DOMAIN" = "$ADMIN_MAILBOX" ] || Z alias add "$a@$DOMAIN" "$ADMIN_MAILBOX" >/dev/null 2>&1 || true
  done
  umask 077
  cat > /root/tmail-credentials.txt <<EOF
TMAIL — $(date '+%F %T')
Webmail:            https://$HOSTNAME_/
Administrator:      $ADMIN_LOGIN / $ADMIN_PASS
Mailbox:            $ADMIN_MAILBOX / $MAILBOX_PASS  (for phone and desktop mail programs)
Backup password:    $(envget BACKUP_PASSWORD)  (keep it outside this server: backups cannot be read without it)
EOF
  ok "administrator and mailbox created (passwords in /root/tmail-credentials.txt)"
fi

# ---------- 7. firewall, fail2ban, watchdog, backups ----------
step "Protection, watchdog and backups"
SSH_PORT=$(ss -Hltnp 2>/dev/null | awk '/sshd/{split($4,a,":"); print a[length(a)]; exit}'); SSH_PORT=${SSH_PORT:-22}
if command -v nft >/dev/null; then
  sed "s/tcp dport 22 /tcp dport $SSH_PORT /g" deploy/firewall.nft > /etc/tmail-firewall.nft
  nft -c -f /etc/tmail-firewall.nft && nft -f /etc/tmail-firewall.nft
  grep -q tmail-firewall /etc/nftables.conf 2>/dev/null || echo 'include "/etc/tmail-firewall.nft"' >> /etc/nftables.conf
  systemctl enable nftables >/dev/null 2>&1 || true
  ok "firewall: only mail and web ports are open (SSH on $SSH_PORT)"
fi
if [ -d /etc/fail2ban ]; then
  cp -r deploy/fail2ban/. /etc/fail2ban/
  sed -i "s#^ignoreip = .*#ignoreip = 127.0.0.1/8 ::1 $PUBLIC_IP#" /etc/fail2ban/jail.d/zmail.conf
  systemctl restart fail2ban >/dev/null 2>&1 && ok "fail2ban: password guessing is blocked"
fi
install -m 755 deploy/watchdog/tmail-watchdog.sh /usr/local/sbin/tmail-watchdog.sh
sed "s#^ExecStart=.*#Environment=TMAIL_DIR=$DIR\nExecStart=/usr/local/sbin/tmail-watchdog.sh#" deploy/watchdog/tmail-watchdog.service > /etc/systemd/system/tmail-watchdog.service
cp deploy/watchdog/tmail-watchdog.timer /etc/systemd/system/
sed "s#/opt/zmail#$DIR#g" deploy/backup/tmail-backup.service > /etc/systemd/system/tmail-backup.service
cp deploy/backup/tmail-backup.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now tmail-watchdog.timer tmail-backup.timer >/dev/null 2>&1
ok "watchdog every 5 minutes, backup every night at 03:30 → $BACKUP_REPO"

# ---------- 8. DNS ----------
step "DNS records to add at your domain registrar"
DKIM=$(Z dns "$DOMAIN" 2>/dev/null | sed 's/^/    /' || true)
cat <<EOF
    $HOSTNAME_.          A      $PUBLIC_IP
    $FILES_HOST.         A      $PUBLIC_IP
    $DOMAIN.             MX 10  $HOSTNAME_.
    $DOMAIN.             TXT    "v=spf1 mx -all"
    _dmarc.$DOMAIN.      TXT    "v=DMARC1; p=quarantine; rua=mailto:postmaster@$DOMAIN"
EOF
echo "$DKIM" | grep -i dkim || echo "$DKIM"
echo "    and at the hosting provider: reverse DNS (PTR) of $PUBLIC_IP → $HOSTNAME_"
resolved=$(getent ahostsv4 "$HOSTNAME_" 2>/dev/null | awk '{print $1; exit}')
if [ "$resolved" = "$PUBLIC_IP" ]; then ok "$HOSTNAME_ already points here: the HTTPS certificate is issued automatically within minutes"
else warn "$HOSTNAME_ does not point to $PUBLIC_IP yet — once it does, the certificate is issued automatically (no need to run this again)"; fi
echo "    The administration panel (https://$HOSTNAME_/admin → Domains) checks every record live."

step "Done"
echo "  Webmail:  https://$HOSTNAME_/"
[ -n "$ADMIN_PASS" ] && echo "  Login:    $ADMIN_LOGIN / $ADMIN_PASS   (also in /root/tmail-credentials.txt)"
echo "  Mail programs: IMAP $HOSTNAME_:993 (SSL), SMTP $HOSTNAME_:465 (SSL) or 587 (STARTTLS), login = full address"
echo "  Keep the backup password from /root/tmail-credentials.txt somewhere outside this server."
