FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/zmaild ./cmd/zmaild

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates media-types tzdata hunspell hunspell-ru hunspell-en-us \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/zmaild /usr/local/bin/zmaild
EXPOSE 80 443
CMD ["zmaild"]
