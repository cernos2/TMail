#!/bin/bash
# Backup of TMAIL with restic: configuration (.env, compose, services), the database (consistent
# dump), mail, DKIM keys, certificates, spam-filter data and the large-file storage.
# Incremental and deduplicated: the first run copies everything, later runs only what changed, so a
# daily backup of tens of gigabytes of mail is fast and small. Encrypted with BACKUP_PASSWORD.
#
# Where the backups go (BACKUP_REPOSITORY in .env):
#   /var/backups/tmail/restic         a folder on this server (default) — protects from mistakes,
#                                     not from losing the server: prefer one of the following
#   s3:https://s3.example.com/bucket  any S3 storage (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY in .env)
#   sftp:user@host:/path              another server over SSH (key in /root/.ssh)
# Keeps 7 daily, 4 weekly and 3 monthly snapshots. Restore with deploy/restore.sh.
# Usage (on the server, in /opt/zmail):  deploy/backup.sh
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT=$(pwd)
set -a; . ./.env; set +a
: "${BACKUP_PASSWORD:?BACKUP_PASSWORD is missing in .env (openssl rand -hex 24) — keep a copy of it outside this server}"
REPO="${BACKUP_REPOSITORY:-/var/backups/tmail/restic}"
RESTIC_IMAGE=restic/restic:0.18.1
P=$(docker compose config --format json 2>/dev/null | python3 -c 'import json,sys; print(json.load(sys.stdin)["name"])' 2>/dev/null || echo zmail)
VOLS=$(dirname "$(dirname "$(docker volume inspect -f '{{.Mountpoint}}' "${P}_vmail")")")
STAGE=/var/backups/tmail/stage # on disk: /tmp is often a small RAM disk
mkdir -p "$STAGE"
trap 'rm -f "$STAGE/db.dump"' EXIT

restic() {
  local repo_mount=() repo="$REPO"
  if [[ "$REPO" == /* ]]; then mkdir -p "$REPO"; repo_mount=(-v "$REPO:/repo"); repo=/repo; fi
  docker run --rm -i --hostname tmail --cpus 1.5 \
    -e RESTIC_REPOSITORY="$repo" -e RESTIC_PASSWORD="$BACKUP_PASSWORD" \
    -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-}" -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-}" \
    -v restic-cache:/root/.cache/restic -v /root/.ssh:/root/.ssh:ro \
    "${repo_mount[@]}" -v "$ROOT:/src/config:ro" -v "$VOLS:/src/volumes:ro" -v "$STAGE:/src/stage:ro" \
    "$RESTIC_IMAGE" "$@"
}

echo "$(date '+%F %T') backup to $REPO"
restic cat config >/dev/null 2>&1 || { echo "• new repository"; restic init; }

echo "• database"
docker compose exec -T postgres pg_dump -U zmail -Fc zmail </dev/null > "$STAGE/db.dump"

# Volumes are read from the host; the database is in the dump, previews and sockets are rebuilt.
paths=(/src/config /src/stage)
for v in vmail dkim letsencrypt tls rspamddata redisdata garage-meta garage-data postfixspool; do
  [ -d "$VOLS/${P}_$v/_data" ] && paths+=("/src/volumes/${P}_$v/_data")
done
echo "• files"
# Search indexes of the mail are rebuilt by Dovecot by themselves: not worth gigabytes of backup.
restic backup --compression auto --exclude 'xapian-indexes' --exclude '/src/config/web/node_modules' \
  --exclude '/src/config/.git' --tag tmail "${paths[@]}"

echo "• old snapshots"
restic forget --tag tmail --keep-daily 7 --keep-weekly 4 --keep-monthly 3 --prune
# Once a week: read a part of the stored data back, so a damaged repository is noticed in time.
if [ "$(date +%u)" = 7 ]; then restic check --read-data-subset=5%; fi
restic snapshots --tag tmail --compact | tail -n 4
echo "$(date '+%F %T') done"
