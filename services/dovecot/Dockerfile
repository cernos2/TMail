FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends dovecot-core dovecot-imapd dovecot-pop3d dovecot-lmtpd \
      dovecot-pgsql dovecot-sieve dovecot-managesieved dovecot-fts-xapian poppler-utils catdoc unzip file ca-certificates gettext-base \
 && rm -rf /var/lib/apt/lists/* \
 && test -f /usr/share/dovecot/dh.pem \
 && groupadd -g 5000 vmail && useradd -u 5000 -g vmail -d /var/vmail -s /usr/sbin/nologin vmail \
 && mkdir -p /var/vmail /etc/dovecot/sieve/before.d && chown vmail:vmail /var/vmail \
 && rm -rf /etc/dovecot/conf.d
COPY dovecot.conf.tmpl dovecot-sql.conf.ext.tmpl /etc/dovecot/
COPY sieve/ /etc/dovecot/sieve/before.d/
COPY decode2text.sh /usr/local/bin/decode2text.sh
COPY entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh
EXPOSE 143 993 995 4190
ENTRYPOINT ["/entrypoint.sh"]
