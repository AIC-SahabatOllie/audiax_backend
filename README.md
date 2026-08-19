# audiax_backend

Go backend built on the clean-architecture layering of
[khannedy/golang-clean-architecture](https://github.com/khannedy/golang-clean-architecture),
adapted for Supabase Postgres and Redis-backed sessions.

## Architecture

```
HTTP request
  -> delivery/http        parse body into a model.Request, nothing else
  -> usecase              validate, apply business rules, orchestrate
  -> repository           entity + *gorm.DB (plain connection or transaction)
  -> Postgres / Redis
  -> model/converter      entity -> model.Response (never leaks password_hash)
```

Two rules hold the layering together:

- **`*gorm.DB` is passed into repositories, never stored.** A use case can hand
  down either the pooled connection or an open transaction without the
  repository knowing which.
- **Use cases never import a transport package.** They return the sentinels in
  `internal/apperr`; `internal/delivery/http/errors.go` is the only place that
  turns those into status codes. The same use case works from gRPC, a CLI or a
  worker unchanged.

### Layout

| Path | Responsibility |
|---|---|
| `cmd/web` | process lifecycle: load config, connect, serve, drain |
| `internal/app` | composition root — wires concrete types together |
| `internal/config` | infrastructure constructors, leaf package (no app imports) |
| `internal/apperr` | the errors the business layer is allowed to speak |
| `internal/entity` | database rows |
| `internal/model` | request/response DTOs, plus `converter` |
| `internal/repository` | persistence; `repository.go` holds the generic base |
| `internal/usecase` | business rules; declares the repository interfaces it needs |
| `internal/delivery/http` | controllers, auth middleware, routes, error mapping |
| `db/migrations` | golang-migrate SQL |

## Auth

Sessions live in Redis, not in a database column:

```
login  -> bcrypt compare -> 32 random bytes -> token
       -> SET session:<sha256(token)> = userID  (TTL = SESSION_TTL)
auth   -> GET session:<sha256(token)>           (no database round trip)
logout -> DEL
```

- Only the SHA-256 digest is stored, so a leaked Redis dump cannot be replayed.
- A password change calls `DeleteAllForUser`, revoking every outstanding token.
- An unknown email still runs a bcrypt comparison, so response time does not
  reveal which addresses are registered.

## Setup

```bash
cp .env.example .env      # then fill in DATABASE_URL and REDIS_URL
go mod download
```

`DATABASE_URL` comes from Supabase Dashboard -> Connect. Three forms work:

| Form | Host | Note |
|---|---|---|
| Direct | `db.<ref>.supabase.co:5432` | IPv6 only — fails on IPv4-only networks |
| Session pooler | `aws-<n>-<region>.pooler.supabase.com:5432` | IPv4, prepared statements kept |
| Transaction pooler | `aws-<n>-<region>.pooler.supabase.com:6543` | IPv4; the app detects `:6543` and disables prepared statements automatically |

`REDIS_URL` must use the `rediss://` scheme for Upstash and other TLS-only hosts.

### Migrations

Requires [golang-migrate](https://github.com/golang-migrate/migrate).

```bash
make migrate-up
make migrate-new name=create_table_xxx
```

The `users` migration enables row-level security and revokes `anon` /
`authenticated` access, so the table holding password hashes is unreachable
through the Supabase Data API. The backend connects as the owner role, which
bypasses RLS.

## Commands

```bash
make run     # go run ./cmd/web
make test    # go test ./... -race
make lint    # go vet + gofmt
make build   # bin/web
make docker  # distroless image
```

## API

| Method | Path | Auth | Response |
|---|---|---|---|
| `GET` | `/healthz` | — | `200` |
| `POST` | `/api/users` | — | `201` user |
| `POST` | `/api/users/_login` | — | `200` token + expiry |
| `GET` | `/api/users/_current` | Bearer | `200` user |
| `PATCH` | `/api/users/_current` | Bearer | `200` user |
| `DELETE` | `/api/users/_current` | Bearer | `204` |

Success bodies are `{"data": ...}`. Failures are `{"error": "...", "fields": {...}}`,
where `fields` appears only on validation errors:

```json
{
  "error": "validation failed",
  "fields": {
    "email": "must be a valid email address",
    "password": "must be at least 8 characters"
  }
}
```

## Deviations from the upstream template

| Upstream | Here | Why |
|---|---|---|
| Kafka producer + `cmd/worker` | removed | no consumer exists yet |
| `config.json` committed with credentials | env vars + `.env` | 12-factor, per-environment |
| viper, logrus | `os.Getenv` + `log/slog` | stdlib covers it; ~30 fewer dependencies |
| `fiber.ErrBadRequest` from use cases | `internal/apperr` sentinels | business layer stays transport-free |
| validation errors discarded | per-field response | clients can act on the failure |
| token column on `users`, unindexed | Redis session store | was a full table scan per authenticated request |
| `db.Save()` on update | `db.Updates(fields)` | `Save` rewrites every column |
| transaction opened on read paths | reads use the plain connection | one fewer transaction per request |
| `app.Listen()` bare | `ShutdownWithTimeout` | in-flight requests survive a deploy |
| `signal.Notify(..., SIGKILL)` | `SIGINT` + `SIGTERM` | SIGKILL cannot be caught |
| `App.Use(auth)` | scoped route group | `Use` silently locks down routes added later |
| tests require live MySQL + Kafka | use cases unit-tested with fakes | the interfaces live at the consumer |
