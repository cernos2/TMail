FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
RUN echo "postfix postfix/main_mailer_type select No configuration" | debconf-set-selections \
 && apt-get update \
 && apt-get install -y --no-install-recommends postfix postfix-pgsql postfix-pcre ca-certificates gettext-base \
 && rm -rf /var/lib/apt/lists/*
COPY main.cf master.cf submission_header_cleanup /etc/postfix/
COPY pgsql/ /etc/postfix/pgsql/
COPY entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh \
 && chmod 644 /etc/postfix/main.cf /etc/postfix/master.cf /etc/postfix/submission_header_cleanup /etc/postfix/pgsql/*.tmpl
EXPOSE 25 465 587
ENTRYPOINT ["/entrypoint.sh"]
