#!/usr/bin/env bash
#
# Run the service against the development environment, without live reload.
#
# THIS FILE AND .air.toml MUST AGREE. They are two ways to start the same
# service against the same stack, and when they drift the one you did not use
# is the one that hides a bug. They had drifted: this script ran the IP rate
# limiter at 200 req/s burst 400 while .air.toml ran the shipped 100/300, so a
# limit that held under `air` could pass here and vice versa.
#
# The rule is the same one that removed the old 24h access-token override:
# a dev stack that disagrees with production hides exactly the bugs production
# will have. Where a value has a shipped default, dev runs the shipped default.
# (The token lifetimes are a database row now, seeded at 5m / 24h and edited
# through PUT /auth/token_lifetimes; there is no flag to override.)
#
# Prefer `air` for day-to-day work -- it rebuilds on save. Use this script when
# you want a plain `go run` with no file watching, e.g. under a debugger.
#
# Requires: make start-dev-env (PostgreSQL, Valkey, Mailpit, Grafana stack)

set -euo pipefail

go run cmd/go-rest-api-service-template/main.go \
  -log.level=ctrace \
  -log.add.source=true \
  -opentelemetry.trace.exporter=otlp-http \
  -opentelemetry.metric.exporter=otlp-http \
  -authn.private.key.file=./certs/jwt.key \
  -authn.public.key.file=./certs/jwt.pub \
  -authn.symmetric.key.file=./certs/aes-256-symmetric-hex.key \
  -http.server.pprof.enabled=false \
  -http.server.cors.enabled=true \
  -http.server.cors.allowed.origins="*" \
  -http.server.trusted.proxies= \
  -http.server.address=0.0.0.0 \
  -http.server.port=8080 \
  -http.server.tls.enabled=false \
  -http.server.tls.cert.file=./certs/goapitemplate.local.crt \
  -http.server.tls.key.file=./certs/goapitemplate.local.key \
  -mail.smtp.host=localhost \
  -mail.smtp.port=1025 \
  -mail.smtp.username=welcome@goapitemplate.local \
  -mail.smtp.password=new_secure_password \
  -mail.smtp.require.tls=false \
  -cache.enabled=true \
  -ratelimit.enabled=true \
  -cache.tls.enabled=true \
  -cache.tls.ca.file=./certs/dev/ca.crt \
  -cache.client.enabled=true \
  -database.ssl.mode=verify-full \
  -database.ssl.root.cert.file=./certs/dev/ca.crt
