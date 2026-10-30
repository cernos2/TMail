#!/bin/bash
# Builds the web interface on the server itself (Node in a container, nothing installed on the host)
# and publishes it into web/dist *in place*: the directory is bind-mounted into the web container,
# so it must never be deleted. Old /_nuxt files stay 14 days: a tab opened before an update still
# loads the parts it asks for.
set -euo pipefail
cd "$(dirname "$0")/.."
docker run --rm -v "$PWD/web:/app" -w /app node:22.12-alpine sh -c 'npm ci --no-audit --no-fund --loglevel=error && npx nuxt generate >/dev/null'
mkdir -p web/dist
find web/dist -mindepth 1 -not -path 'web/dist/_nuxt*' -delete
find web/dist/_nuxt -type f -mtime +14 -delete 2>/dev/null || true
cp -a web/.output/public/. web/dist/
echo "web interface published to web/dist"
