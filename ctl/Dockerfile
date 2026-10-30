FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/zmail ./cmd/zmail

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/zmail /usr/local/bin/zmail
ENTRYPOINT ["zmail"]
