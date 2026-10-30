FROM alpine:3.22
RUN apk add --no-cache certbot openssl
COPY run.sh /run.sh
RUN chmod 755 /run.sh
CMD ["/run.sh"]
