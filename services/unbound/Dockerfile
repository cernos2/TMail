FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends unbound dns-root-data ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir -p /var/lib/unbound && cp /usr/share/dns/root.key /var/lib/unbound/root.key \
 && chown -R unbound:unbound /var/lib/unbound
COPY unbound.conf /etc/unbound/unbound.conf
CMD ["unbound", "-d", "-p", "-c", "/etc/unbound/unbound.conf"]
