FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/preview .

FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
# UTF-8 file names: sheets and documents named in Russian are written as «????» otherwise.
ENV LANG=C.UTF-8 LC_ALL=C.UTF-8
RUN apt-get update \
 && apt-get install -y --no-install-recommends libreoffice-writer-nogui libreoffice-calc-nogui libreoffice-impress-nogui poppler-utils tesseract-ocr tesseract-ocr-rus tesseract-ocr-eng \
      fonts-dejavu fonts-liberation fonts-liberation2 fonts-crosextra-carlito fonts-crosextra-caladea \
      python3-uno python3-pip \
 # unoserver keeps one LibreOffice running, so a conversion takes a fraction of a second
 # instead of starting LibreOffice (5–7 s) for every document.
 && pip3 install --no-cache-dir --break-system-packages unoserver==2.2.2 \
 && apt-get purge -y python3-pip && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/* \
 && useradd -m -u 11400 preview && mkdir -p /run/preview && chown preview /run/preview
COPY --from=build /out/preview /usr/local/bin/preview
USER preview
CMD ["preview"]
