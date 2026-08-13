FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/v2ray-scrapper .

FROM alpine:3.22 AS sing-box
ARG TARGETARCH
ARG SING_BOX_VERSION=1.13.12
RUN apk add --no-cache ca-certificates tar wget \
    && case "$TARGETARCH" in \
         amd64) checksum=1540533adb3df24f5ad5f14b5c7ca3dbc2401b10a1c1eb278fcadcada47ec6c4 ;; \
         arm64) checksum=1ffa3b48ad6fa98f9fd810482e39bdd5b6157782ef11ce37d67bdcfd9338547a ;; \
         *) exit 1 ;; \
       esac \
    && archive="sing-box-${SING_BOX_VERSION}-linux-${TARGETARCH}.tar.gz" \
    && wget -q -O "/tmp/${archive}" "https://github.com/SagerNet/sing-box/releases/download/v${SING_BOX_VERSION}/${archive}" \
    && echo "${checksum}  /tmp/${archive}" | sha256sum -c - \
    && mkdir -p /sing-box \
    && tar -xzf "/tmp/${archive}" --strip-components=1 -C /sing-box

FROM alpine:3.22
RUN apk add --no-cache ca-certificates gcompat git tzdata \
    && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/v2ray-scrapper /usr/local/bin/v2ray-scrapper
COPY --from=sing-box /sing-box/sing-box /usr/local/bin/sing-box
COPY src/Country.mmdb /app/Country.mmdb
COPY config.yaml.sample /app/config.yaml
RUN mkdir -p /data && chown app:app /data
USER app
ENV LISTEN_ADDR=0.0.0.0:8084 \
    STATE_FILE_PATH=/data/state.json \
    GEOIP_DB_PATH=/app/Country.mmdb \
    SING_BOX_PATH=/usr/local/bin/sing-box
VOLUME ["/data"]
EXPOSE 8084
ENTRYPOINT ["v2ray-scrapper"]
