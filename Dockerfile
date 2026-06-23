FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/serverlist-go ./src

FROM alpine:3.22

RUN apk add --no-cache ca-certificates su-exec

ENV SERVERLIST_HOST=0.0.0.0 \
    SERVERLIST_PORT=5000 \
    SERVERLIST_CONFIG=/data/config.scfg \
    SERVERLIST_DATA_DIR=/data \
    SERVERLIST_GEOIP_DATABASE=/data/dbip-country-lite.mmdb \
    SERVERLIST_DOWNLOAD_GEOIP=true \
    SERVERLIST_REQUIRE_GEOIP=false \
    SERVERLIST_UID=1000 \
    SERVERLIST_GID=1000

WORKDIR /app
COPY --from=build /out/serverlist-go /usr/local/bin/serverlist-go
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
COPY config.example.scfg /app/config.example.scfg

VOLUME ["/data"]
EXPOSE 5000

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["serverlist-go"]
