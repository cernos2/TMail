#!/bin/bash
# One-time setup of the Garage storage on a new server: layout, access key and bucket.
# Writes S3_ACCESS_KEY / S3_SECRET_KEY into .env. Safe to run again (skips what exists).
set -euo pipefail
cd "$(dirname "$0")/.."
G() { docker compose exec -T garage /garage "$@" </dev/null; }
for i in $(seq 1 30); do G status >/dev/null 2>&1 && break; sleep 2; done
NODE=$(G status 2>/dev/null | awk '/^[0-9a-f]{16}/{print $1; exit}')
if G layout show 2>/dev/null | grep -q "No nodes"; then
  CAP=$(df -BG --output=avail /var/lib/docker | tail -1 | tr -dc 0-9)
  G layout assign -z dc1 -c "$((CAP * 8 / 10))G" "$NODE"
  G layout apply --version 1
fi
if ! grep -q '^S3_ACCESS_KEY=.' .env; then
  OUT=$(G key create tmail-files-key)
  ID=$(echo "$OUT" | awk '/Key ID/{print $NF}')
  SECRET=$(echo "$OUT" | awk '/Secret key/{print $NF}')
  printf 'S3_ACCESS_KEY=%s\nS3_SECRET_KEY=%s\n' "$ID" "$SECRET" >> .env
fi
G bucket create tmail-files 2>/dev/null || true
KEY=$(grep '^S3_ACCESS_KEY=' .env | cut -d= -f2)
G bucket allow --read --write --owner tmail-files --key "$KEY"
G bucket info tmail-files | head -5
