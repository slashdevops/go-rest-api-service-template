#!/usr/bin/env bash
#
# Generate the TLS material the development environment uses for PostgreSQL and
# Valkey.
#
# Development runs with TLS on deliberately. The alternative — shipping the
# capability and never exercising it — is exactly how the gob encoder default
# reached production broken: nothing anyone ran used it. A TLS path that only
# executes in production is a TLS path nobody has tested.
#
# One CA and one server certificate serve both containers; the SANs cover every
# name each is reached by, from inside the pod and from the host.
#
# Usage: generate-dev-certs.sh <output-directory>

set -euo pipefail

OUT="${1:?usage: generate-dev-certs.sh <output-directory>}"

mkdir -p "$OUT"

# Idempotent: regenerating on every `make start-dev-env` would invalidate the
# CA the running service already loaded, and rotating a dev CA daily helps
# nobody. Only regenerate when the certificate is missing or has expired.
if [ -f "$OUT/server.crt" ] && openssl x509 -checkend 86400 -noout -in "$OUT/server.crt" >/dev/null 2>&1; then
  echo "  dev TLS certificates are present and valid, skipping"
  exit 0
fi

echo "  generating dev TLS certificates in $OUT"

# prime256v1 matches the curve used elsewhere in this repository.
openssl ecparam -genkey -name prime256v1 -noout -out "$OUT/ca.key" 2>/dev/null
openssl req -x509 -new -key "$OUT/ca.key" -sha256 -days 365 \
  -out "$OUT/ca.crt" -subj "/CN=go-rest-api-template-dev-ca" 2>/dev/null

openssl ecparam -genkey -name prime256v1 -noout -out "$OUT/server.key" 2>/dev/null
openssl req -new -key "$OUT/server.key" -out "$OUT/server.csr" \
  -subj "/CN=localhost" 2>/dev/null

# Every name the servers are reached by. "postgres" and "valkey" are the
# container names inside the pod; localhost and 127.0.0.1 are how the service
# and the integration suite reach them from the host. A certificate without
# these fails verification even though it is otherwise valid — the single most
# common reason a correctly generated certificate still will not connect.
cat > "$OUT/server.ext" <<EXT
subjectAltName = DNS:localhost, DNS:postgres, DNS:valkey, IP:127.0.0.1
extendedKeyUsage = serverAuth
EXT

openssl x509 -req -in "$OUT/server.csr" -CA "$OUT/ca.crt" -CAkey "$OUT/ca.key" \
  -CAcreateserial -out "$OUT/server.crt" -days 365 -sha256 \
  -extfile "$OUT/server.ext" 2>/dev/null

# PostgreSQL refuses to start if the key is group- or world-readable.
chmod 600 "$OUT/server.key" "$OUT/ca.key"
chmod 644 "$OUT/server.crt" "$OUT/ca.crt"

rm -f "$OUT/server.csr" "$OUT/server.ext"

echo "  done: $(basename "$OUT")/{ca.crt,server.crt,server.key}"
