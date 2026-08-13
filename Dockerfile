FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/v2ray-scrapper .

FROM alpine:3.22 AS xray
ARG TARGETARCH
RUN apk add --no-cache ca-certificates unzip wget \
    && case "$TARGETARCH" in amd64) archive=Xray-linux-64.zip ;; arm64) archive=Xray-linux-arm64-v8a.zip ;; *) exit 1 ;; esac \
    && wget -q -O /tmp/xray.zip "https://github.com/XTLS/Xray-core/releases/latest/download/${archive}" \
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
