#!/bin/bash
# Development helper: builds the Nuxt app here and publishes it to the server DEPLOY_HOST *in place*: web/dist is bind-mounted into the
# web container, so the directory itself must never be deleted (the container would keep the old one).
# Old /_nuxt files stay 14 days: a tab opened before an update still loads the parts it asks for.
set -euo pipefail
cd "$(dirname "$0")/.."
source ~/.nvm/nvm.sh >/dev/null && nvm use 22 >/dev/null
(cd web && npx nuxt generate >/dev/null)
export SSHPASS="${SSHPASS:?set SSHPASS}"
O="-o StrictHostKeyChecking=accept-new -o ConnectTimeout=30 -o PreferredAuthentications=password -o PubkeyAuthentication=no"
tar czf - -C web/.output/public . | sshpass -e ssh $O root@"${DEPLOY_HOST:?set DEPLOY_HOST}" \
  'set -e; mkdir -p /opt/zmail/web/dist /tmp/zmail-web.new; rm -rf /tmp/zmail-web.new/*; tar xzf - -C /tmp/zmail-web.new;
   find /opt/zmail/web/dist -mindepth 1 -not -path "/opt/zmail/web/dist/_nuxt*" -delete;
   find /opt/zmail/web/dist/_nuxt -type f -mtime +14 -delete 2>/dev/null || true;
   cp -a /tmp/zmail-web.new/. /opt/zmail/web/dist/; rm -rf /tmp/zmail-web.new; echo published'
