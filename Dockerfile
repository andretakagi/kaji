# Stage 1: Caddy binary from official image
FROM caddy:2.9.1-alpine AS caddy

# Stage 2: Build frontend
FROM oven/bun:1.2.5 AS frontend
WORKDIR /build
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ .
RUN bun run build

# Stage 3: Build Go binary
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /dist ./dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o kaji .

# Stage 4: Final image
FROM alpine:3.23
RUN apk add --no-cache ca-certificates libcap
COPY --from=caddy /usr/bin/caddy /usr/local/bin/caddy
COPY --from=builder /build/kaji /usr/local/bin/kaji
RUN setcap cap_net_bind_service=+ep /usr/local/bin/caddy
RUN addgroup -S kaji && adduser -S -G kaji kaji
RUN mkdir -p /etc/caddy /data /config /etc/caddy-gui /var/log/caddy \
    && chown -R kaji:kaji /etc/caddy /data /config /etc/caddy-gui /var/log/caddy
ENV CADDY_GUI_MODE=docker
EXPOSE 80 443 8080
USER kaji
ENTRYPOINT ["kaji"]
