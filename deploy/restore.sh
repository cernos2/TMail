#!/bin/bash
# Restore TMAIL from a restic backup made by deploy/backup.sh — on this or on a new server.
# Requirements: Docker with the compose plugin, python3, and free disk space for the restored data.
# Usage:
#   BACKUP_REPOSITORY=… BACKUP_PASSWORD=… deploy/restore.sh [snapshot|latest] [/opt/zmail]
#   (for S3 also AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY; for SFTP a key in /root/.ssh)
#   deploy/restore.sh --list      the snapshots in the repository
# Then point DNS (A/MX/PTR) to the new server; certificates renew by themselves.
set -euo pipefail
: "${BACKUP_PASSWORD:?set BACKUP_PASSWORD (the one from .env of the old server)}"
REPO="${BACKUP_REPOSITORY:-/var/backups/tmail/restic}"
RESTIC_IMAGE=restic/restic:0.18.1
WORK=/var/backups/tmail/restore # on disk: the mail does not fit into /tmp

restic() {
  local repo_mount=() repo="$REPO"
  if [[ "$REPO" == /* ]]; then repo_mount=(-v "$REPO:/repo"); repo=/repo; fi
  docker run --rm -i -e RESTIC_REPOSITORY="$repo" -e RESTIC_PASSWORD="$BACKUP_PASSWORD" \
    -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-}" -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-}" \
    -v /root/.ssh:/root/.ssh:ro "${repo_mount[@]}" -v "$WORK:/restore" "$RESTIC_IMAGE" "$@"
}

if [ "${1:-}" = "--list" ]; then mkdir -p "$WORK"; restic snapshots --tag tmail; exit 0; fi
SNAP="${1:-latest}"
DEST="${2:-/opt/zmail}"

rm -rf "$WORK" && mkdir -p "$WORK"
echo "• restoring snapshot $SNAP"
restic restore "$SNAP" --tag tmail --target /restore
SRC="$WORK/src"

echo "• configuration → $DEST"
mkdir -p "$DEST"
cp -a "$SRC/config/." "$DEST/"
chmod 600 "$DEST/.env"
cd "$DEST"
P=$(docker compose config --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["name"])')
docker compose down 2>/dev/null || true

for d in "$SRC"/volumes/*/_data; do
  [ -d "$d" ] || continue
  v=$(basename "$(dirname "$d")"); v=${v#*_} # "<old project>_vmail" → "vmail"
  echo "• volume $v"
  docker volume create "${P}_${v}" >/dev/null
  docker run --rm -v "${P}_${v}:/data" -v "$d:/in:ro" alpine:3.20 sh -c "rm -rf /data/* /data/..?* /data/.[!.]* 2>/dev/null; cp -a /in/. /data/"
done

echo "• building images"
docker compose build
echo "• database"
docker compose up -d postgres
for i in $(seq 1 30); do docker compose exec -T postgres pg_isready -U zmail >/dev/null 2>&1 && break; sleep 2; done
docker compose exec -T postgres pg_restore -U zmail -d zmail --clean --if-exists --no-owner < "$SRC/stage/db.dump" || true
echo "• starting everything"
docker compose up -d
docker compose ps
rm -rf "$WORK"
echo "Done. If the server's IP address changed, update PUBLIC_IP in .env, the DNS records (A, MX, SPF) and PTR at the hosting provider."
