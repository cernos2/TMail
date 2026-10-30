FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
# Rspamd from the project's own repository (Debian's package is several releases behind).
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl gnupg \
 && curl -fsSL https://rspamd.com/apt-stable/gpg.key | gpg --dearmor -o /usr/share/keyrings/rspamd.gpg \
 && echo "deb [signed-by=/usr/share/keyrings/rspamd.gpg] http://rspamd.com/apt-stable/ bookworm main" > /etc/apt/sources.list.d/rspamd.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends rspamd \
 && apt-get purge -y gnupg && apt-get autoremove -y && rm -rf /var/lib/apt/lists/* \
 # Fixed uid/gid so the ctl tool can hand DKIM keys to rspamd.
 && usermod -u 11333 _rspamd && groupmod -g 11333 _rspamd \
 && mkdir -p /var/lib/rspamd/dkim /run/rspamd /var/log/rspamd \
 && chown -R _rspamd:_rspamd /var/lib/rspamd /run/rspamd /var/log/rspamd
COPY local.d/ /etc/rspamd/local.d/
EXPOSE 11332 11333 11334
CMD ["rspamd", "-f", "-u", "_rspamd", "-g", "_rspamd"]
