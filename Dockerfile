FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/v2ray-scrapper .

FROM alpine:3.22 AS xray
ARG TARGETARCH
ARG XRAY_VERSION=v26.3.27
RUN apk add --no-cache ca-certificates unzip wget \
    && case "$TARGETARCH" in \
         amd64) archive=Xray-linux-64.zip; checksum=23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae ;; \
         arm64) archive=Xray-linux-arm64-v8a.zip; checksum=4d30283ae614e3057f730f67cd088a42be6fdf91f8639d82cb69e48cde80413c ;; \
         *) exit 1 ;; \
       esac \
    && wget -q -O /tmp/xray.zip "https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/${archive}" \
    && echo "${checksum}  /tmp/xray.zip" | sha256sum -c - \
    && unzip /tmp/xray.zip -d /xray

FROM alpine:3.22
RUN apk add --no-cache ca-certificates git tzdata \
    && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/v2ray-scrapper /usr/local/bin/v2ray-scrapper
COPY --from=xray /xray/xray /usr/local/bin/xray
COPY --from=xray /xray/geoip.dat /xray/geosite.dat /usr/local/bin/
COPY src/Country.mmdb /app/Country.mmdb
COPY config.yaml.sample /app/config.yaml
RUN mkdir -p /data && chown app:app /data
USER app
ENV LISTEN_ADDR=0.0.0.0:8084 \
    STATE_FILE_PATH=/data/state.json \
    GEOIP_DB_PATH=/app/Country.mmdb \
    XRAY_PATH=/usr/local/bin/xray \
    XRAY_ASSETS_PATH=/usr/local/bin
VOLUME ["/data"]
EXPOSE 8084
ENTRYPOINT ["v2ray-scrapper"]
