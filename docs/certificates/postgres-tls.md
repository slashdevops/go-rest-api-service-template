# PostgreSQL TLS

How to generate the certificates for the database connection, enable TLS on the
server, and choose an SSL mode that actually protects anything.

Every command below was run end to end against `postgres:18` before being
written down.

## What each mode buys

Start here, because the names mislead and most people stop at the wrong one.

| `database.ssl.mode` | Encrypts | Authenticates the server | Verdict |
| --- | --- | --- | --- |
| `disable` | no | no | cleartext. The shipped default |
| `allow` / `prefer` | **maybe** | no | **silently downgrades** to cleartext when the server does not offer TLS, and nothing reports it |
| `require` | yes | **no** | encrypted to *whoever answered*. Does not stop an interception that terminates TLS |
| `verify-ca` | yes | partially | chains to a trusted CA, but the hostname is not checked |
| `verify-full` | yes | yes | the only mode that resists an active attacker |

`prefer` is the trap: it reads like a safe default and guarantees nothing. If
you are turning this on at all, `verify-full` is the setting worth having.

The connection carries every credential in the DSN, every row read or written —
including the password hashes at their source.

## Requirements

- [OpenSSL 3.x](https://www.openssl.org/)
- A PostgreSQL server built with TLS support (the official images are)

## Step 1 — Create a CA

```bash
mkdir -p certs
cd certs

openssl ecparam -genkey -name prime256v1 -noout -out ca.key
openssl req -x509 -new -key ca.key -sha256 -days 365 \
  -out ca.crt -subj "/CN=go-rest-api-template-db-ca"
```

## Step 2 — Create the server certificate

**The Subject Alternative Name is mandatory** for `verify-full`, which checks
the hostname. List every name the server is reached by.

```bash
openssl ecparam -genkey -name prime256v1 -noout -out server.key
openssl req -new -key server.key -out server.csr -subj "/CN=postgres"

cat > server.ext <<'EXT'
subjectAltName = DNS:postgres, DNS:localhost, IP:127.0.0.1
extendedKeyUsage = serverAuth
EXT

openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt -days 365 -sha256 -extfile server.ext

# PostgreSQL refuses to start if the key is group- or world-readable.
chmod 600 server.key
chmod 644 server.crt ca.crt
```

## Step 3 — Enable TLS on the server

```bash
postgres \
  -c ssl=on \
  -c ssl_cert_file=/certs/server.crt \
  -c ssl_key_file=/certs/server.key \
  -c ssl_ca_file=/certs/ca.crt
```

As a container:

```bash
podman run -d --rm --name pg-tls -p 5432:5432 \
  -e POSTGRES_PASSWORD=password -e POSTGRES_USER=username -e POSTGRES_DB=go-rest-api-service-template \
  -v "$PWD/certs":/certs:ro \
  --user 999:999 \
  docker.io/library/postgres:18 \
  -c ssl=on -c ssl_cert_file=/certs/server.crt \
     -c ssl_key_file=/certs/server.key -c ssl_ca_file=/certs/ca.crt
```

**`--user 999:999` is required, not decoration.** The key must be `0600`, and
the server can only read it while running as the uid that owns it. Without this
the container exits during startup.

`ssl=on` makes the server *accept* TLS; it does not *require* it. Requiring it
is a `pg_hba.conf` decision (`hostssl` entries, and no `host` fallback), which
is where you make cleartext impossible rather than merely unused.

## Step 4 — Configure the service

| Flag | Environment variable | Meaning |
| --- | --- | --- |
| `database.ssl.mode` | `DATABASE_SSL_MODE` | One of the modes in the table above |
| `database.ssl.root.cert.file` | `DATABASE_SSL_ROOT_CERT_FILE` | CA that signed the server certificate |
| `database.ssl.cert.file` | `DATABASE_SSL_CERT_FILE` | Client certificate, mutual TLS only |
| `database.ssl.key.file` | `DATABASE_SSL_KEY_FILE` | Key for the client certificate |

```bash
go-rest-api-service-template \
  --database.ssl.mode=verify-full \
  --database.ssl.root.cert.file=./certs/ca.crt
```

Without `database.ssl.root.cert.file`, `verify-ca` and `verify-full` can only
validate against the host trust store — which a privately signed server never
matches. That is why the flag exists: the modes were unusable without it.

### What the configuration refuses

- SSL files configured while `database.ssl.mode` is `disable` — the connection
  would still be cleartext, and a CA file in the config reads as though it is not
- `database.ssl.cert.file` without `database.ssl.key.file`, or the reverse
- any file that cannot be read

pgx then reads the certificates while parsing the connection string, so a wrong
path fails at startup rather than at the first query.

### When the connection is not protected

The service says so once at startup, with a different message per mode:

```text
level=WARN msg="database connection is not encrypted; credentials, password
hashes cross the network in the clear" ssl_mode=disable

level=WARN msg="database SSL mode silently falls back to cleartext when the
server does not offer TLS" ssl_mode=prefer use_instead=verify-full

level=WARN msg="database connection is encrypted but the server is not
authenticated; this does not stop an interception" ssl_mode=require
```

## Verify it works

```bash
# From the server: is TLS on at all?
psql -U username -d go-rest-api-service-template -c "show ssl;"

# Which connections are actually encrypted, and with what?
psql -U username -d go-rest-api-service-template -c \
  "select s.ssl, s.version, count(*) from pg_stat_ssl s
     join pg_stat_activity a on a.pid = s.pid
    where a.usename = 'username' group by 1, 2;"
```

A service connected with `verify-full` shows its whole pool as `t | TLSv1.3`.

Then confirm the service **verifies** rather than merely encrypting: start it
with `verify-full` and no root certificate against a privately signed server. It
must refuse. If it connects, verification is not happening.

## The development environment

`make start-dev-env` generates a CA and server certificate into `certs/dev/` and
starts PostgreSQL with `ssl=on`. `run.sh` and `.air.toml` point the service at
`verify-full`, so **development exercises the verifying path**.

That is deliberate. A TLS path that only runs in production is a path nobody has
tested — the same failure mode that let the gob encoder default reach
production broken, because every development run overrode it.

The integration suite still connects in cleartext. It is a test harness rather
than the service, and `ssl=on` accepts both.

Certificates are regenerated only when missing or expired, so restarting the
environment does not invalidate the CA a running service already loaded.

## Troubleshooting

| Symptom | Cause |
| --- | --- |
| `x509: certificate signed by unknown authority` | `database.ssl.root.cert.file` is missing or names the wrong CA. This is verification working |
| `x509: certificate is valid for X, not Y` | The SAN does not list the host you connected to |
| Container exits at startup | The key is not readable by the server's uid — see `--user 999:999` |
| `server does not support SSL, but SSL was required` | `ssl=on` is missing on the server |
| Connects despite a wrong CA | The mode is `require` or lower, which does not verify |
| `SSL files are configured but database.ssl.mode is disable` | Exactly that — the link would still be cleartext |

## See also

- [Valkey TLS](./valkey-tls.md) — the same for the cache connection
- [Certificates](./certificates.md) — the JWT key pair and the AES key
