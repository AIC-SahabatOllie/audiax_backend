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

## Panduan Setup Lokal (untuk Panitia)

Panduan ini menjelaskan langkah demi langkah untuk menjalankan backend AUDIAX
di komputer lokal, dari nol sampai bisa dites lewat `curl` / Postman.

### 1. Prasyarat

| Alat | Wajib untuk | Cek versi |
|---|---|---|
| [Git](https://git-scm.com/) | clone repo | `git --version` |
| [Go 1.25+](https://go.dev/dl/) | menjalankan tanpa Docker (`make run`), atau `go test` | `go version` |
| [Docker Desktop](https://www.docker.com/products/docker-desktop/) | jalur tercepat (`make compose-up`) — sudah termasuk Docker Compose | `docker compose version` |
| [golang-migrate](https://github.com/golang-migrate/migrate#installation) | menjalankan migrasi skema database | `migrate -version` |

Backend ini **bukan** aplikasi yang berdiri sendiri sepenuhnya — dua servis
eksternal disewa (bukan dijalankan lokal), dan satu servis AI dijalankan dari
repo terpisah:

| Servis | Peran | Wajib untuk apa |
|---|---|---|
| Supabase (Postgres + Storage) | database utama, penyimpanan file audio | start aplikasi, semua endpoint berdata |
| Upstash (Redis) | penyimpanan sesi login | start aplikasi, semua endpoint yang butuh login |
| `audiax_model` (repo terpisah, FastAPI) | kalkulasi baseline & health card dari audio | endpoint `.../baselines` dan `.../inspections` |
| Ollama (opsional, lihat bagian "Advisory" di bawah) | jawaban chat "Teknisi Saku" berbasis LLM | opsional — tanpa ini, advisory tetap jalan pakai jawaban statis |

Jika tim penyelenggara sudah membagikan file `.env` terisi beserta akses ke
`audiax_model` yang berjalan, langsung lompat ke langkah 3.

### 2. Konfigurasi environment

```bash
git clone <url-repo-ini>
cd audiax_backend
cp .env.example .env
```

Isi `.env` yang dihasilkan. Lima variabel ini **wajib** diisi — aplikasi
menolak untuk start (`config.Load` mengembalikan error) jika salah satunya
kosong:

| Variabel | Dari mana | Catatan |
|---|---|---|
| `DATABASE_URL` | Supabase Dashboard -> Connect | lihat tabel bentuk koneksi di bawah |
| `REDIS_URL` | Upstash Dashboard -> Connect | **harus** pakai skema `rediss://`, bukan `redis://` (TLS-only) |
| `AI_SERVICE_URL` | URL tempat `audiax_model` berjalan | default lokal: `http://localhost:8000`. Boleh diisi placeholder agar aplikasi start, tapi endpoint kalibrasi/inspeksi akan gagal sampai servis ini benar-benar hidup |
| `SUPABASE_URL` | Supabase Dashboard -> Settings -> API | tanpa trailing slash |
| `SUPABASE_SERVICE_KEY` | Supabase Dashboard -> Settings -> API -> `service_role` key | **rahasia** — jangan pernah dikirim ke client, hanya dipakai server-side |

`DATABASE_URL` punya tiga bentuk yang didukung Supabase; pilih salah satu:

| Bentuk | Host | Catatan |
|---|---|---|
| Direct | `db.<ref>.supabase.co:5432` | IPv6 only — gagal di jaringan IPv4-only (umum terjadi di kampus/kantor) |
| Session pooler | `aws-<n>-<region>.pooler.supabase.com:5432` | IPv4, prepared statements tetap aktif |
| Transaction pooler | `aws-<n>-<region>.pooler.supabase.com:6543` | IPv4; aplikasi otomatis mendeteksi port `:6543` dan mematikan prepared statements |

Jika koneksi langsung (`db.<ref>.supabase.co`) macet tanpa pesan error yang
jelas, itu hampir selalu jaringan IPv4-only — pindah ke salah satu pooler.

Variabel lain di `.env.example` (`SESSION_TTL`, `BCRYPT_COST`,
`SHUTDOWN_TIMEOUT`, dst.) sudah punya nilai default yang masuk akal dan tidak
perlu diubah untuk menjalankan demo.

### 3. Migrasi database

Wajib dijalankan sekali di awal (dan setiap ada migrasi baru), sebelum
aplikasi dipakai — baik lewat `make run` maupun `make compose-up`, karena
keduanya memakai Postgres yang sama dan tidak menjalankan migrasi secara
otomatis saat start.

```bash
make migrate-up
```

Perintah ini membaca `DATABASE_URL` dari `.env` lewat `MIGRATE_URL` di
Makefile. Migrasi `users` mengaktifkan row-level security dan mencabut akses
`anon` / `authenticated`, sehingga tabel berisi hash password tidak bisa
dijangkau lewat Supabase Data API — backend sendiri konek sebagai role owner,
yang melewati RLS.

```bash
make migrate-down                 # rollback 1 migrasi, kalau perlu
make migrate-new name=create_table_xxx   # bikin migrasi baru
```

### 4. Menjalankan aplikasi

Dua jalur — pilih salah satu.

**Opsi A — Docker Compose (direkomendasikan untuk panitia).** Satu perintah,
sudah termasuk servis Ollama untuk fitur advisory:

```bash
make compose-up          # build + start backend & ollama
docker compose logs -f   # pantau log; Ctrl+C untuk berhenti memantau (container tetap jalan)
make compose-down        # matikan semuanya
```

`make compose-up` otomatis menjalankan `scripts/prepare_ollama_model.sh`
lebih dulu, yang menyalin file model GGUF dari `../audiax_model` (lihat
bagian "Advisory ('Teknisi Saku') and Ollama" di bawah untuk detail dan cara
mengatasi kalau gagal). Backend akan tersedia di `http://localhost:3000`.

**Opsi B — langsung dengan Go, tanpa Docker.** Lebih cepat untuk
iterasi/debug, tidak menjalankan servis Ollama:

```bash
go mod download
make run                 # go run ./cmd/web, listen di :3000 (atau $PORT)
```

### 5. Verifikasi aplikasi berjalan

```bash
curl http://localhost:3000/healthz
# -> 200 OK
```

Smoke test lengkap (register lalu login):

```bash
curl -X POST http://localhost:3000/api/users \
  -H "Content-Type: application/json" \
  -d '{"email":"panitia@example.com","password":"password123","name":"Panitia"}'

curl -X POST http://localhost:3000/api/users/_login \
  -H "Content-Type: application/json" \
  -d '{"email":"panitia@example.com","password":"password123"}'
# -> 200 dengan token; pakai token itu sebagai "Authorization: Bearer <token>"
# di endpoint lain (lihat tabel API di bawah)
```

Endpoint `.../baselines` dan `.../inspections` butuh `audiax_model` benar-benar
hidup di `AI_SERVICE_URL`; tanpa itu keduanya akan gagal meski aplikasi ini
sendiri sudah start dengan normal.

### 6. Troubleshooting umum

| Gejala | Penyebab | Solusi |
|---|---|---|
| Aplikasi langsung exit saat start dengan pesan `... is required` | salah satu dari lima variabel wajib di `.env` kosong | isi variabel yang disebut di pesan error |
| Koneksi database timeout tanpa error jelas | `DATABASE_URL` pakai host direct (`db.<ref>.supabase.co`) di jaringan IPv4-only | ganti ke session atau transaction pooler |
| Redis error TLS handshake | `REDIS_URL` pakai `redis://` bukan `rediss://` | ganti skema ke `rediss://` |
| `make migrate-up` gagal `command not found: migrate` | golang-migrate belum terpasang | ikuti link instalasi di tabel prasyarat |
| `docker compose build` gagal di servis `ollama`, atau `prepare_ollama_model.sh` error "not found" | file GGUF (261 MB) belum ada — repo ini sengaja tidak menyimpannya | lihat bagian "Advisory ('Teknisi Saku') and Ollama" di bawah untuk cara menyediakannya, atau set `AUDIAX_MODEL_REPO` |
| Endpoint kalibrasi/inspeksi selalu gagal walau aplikasi jalan normal | `audiax_model` (repo FastAPI terpisah) belum dijalankan, atau `AI_SERVICE_URL` salah | jalankan `audiax_model` di port yang sesuai, cocokkan dengan `AI_SERVICE_URL` |
| Fitur advisory ("Teknisi Saku") menjawab tapi `source` selalu `"fallback_static"` | ini **bukan bug** — Ollama tidak wajib. Lihat bagian "Advisory" di bawah kalau memang ingin jawaban `"llm"` | pastikan `OLLAMA_URL` terisi dan servis Ollama benar-benar hidup |

## Commands

```bash
make run     # go run ./cmd/web
make test    # go test ./... -race
make lint    # go vet + gofmt
make build   # bin/web
make docker  # distroless image
```

## Advisory ("Teknisi Saku") and Ollama

The advisory feature explains a health card and answers operator follow-up
questions using a Gemma model, LoRA-fine-tuned in the sibling `audiax_model`
repo, served locally through [Ollama](https://ollama.com) -- offline, no
third-party API call at runtime. Full design: `audiax_model`'s
`experiments/advisory/DESIGN.md`.

The feature works without Ollama at all: `OLLAMA_URL` unset, unreachable, or
the model unavailable all degrade the same way -- `POST .../advisory/messages`
still answers `200` with `source: "fallback_static"`, built straight from
`internal/advisory/decision_table.json` with no LLM involved (DESIGN.md
decision 8). Wiring up Ollama upgrades `source` to `"llm"`; it is never
required for the endpoint to work.

### Running it

```bash
cp .env.example .env      # OLLAMA_URL defaults to http://localhost:11434

# Option A -- host-installed Ollama, for local dev
make ollama-model         # stages the GGUF from ../audiax_model into ollama/model/
ollama serve &
(cd ollama && ollama create audiax-advisor -f Modelfile)
make run

# Option B -- everything in Docker, no local Ollama install needed
make compose-up      # stages the GGUF, builds both images, starts both containers
docker compose logs -f
make compose-down
```

`make compose-up` (and `compose-build`) first run
`scripts/prepare_ollama_model.sh`, which copies
`audiax-advisor-q4km.gguf` (261 MB) from
`../audiax_model/experiments/advisory/dist/` into `ollama/model/` -- this
assumes both repos are cloned as siblings; override with
`AUDIAX_MODEL_REPO=/path/to/audiax_model`. The script fails loudly (not
silently) if the file is missing or truncated, and the `ollama` image's build
itself fails if `ollama create` can't load it -- neither surfaces as a broken
container at demo time. The GGUF is never committed to either repo; if it's
missing entirely, the script prints the exact `audiax_model` commands that
regenerate it.

`docker-compose.yml`'s `ollama` service bakes the model into the image at
build time and publishes no ports -- only the `backend` container reaches it,
over the compose network, at `http://ollama:11434`. Do not add a volume mount
over `/root/.ollama` there; it would shadow the baked-in model exactly the way
a stray volume mount once shadowed the AI repo's checkpoint (see that repo's
`docs/PROGRESS.md`).

## API

| Method | Path | Auth | Response |
|---|---|---|---|
| `GET` | `/healthz` | — | `200` |
| `POST` | `/api/users` | — | `201` user |
| `POST` | `/api/users/_login` | — | `200` token + expiry |
| `GET` | `/api/users/_current` | Bearer | `200` user |
| `PATCH` | `/api/users/_current` | Bearer | `200` user |
| `DELETE` | `/api/users/_current` | Bearer | `204` |
| `POST` | `/api/machines` | Bearer | `201` machine |
| `GET` | `/api/machines` | Bearer | `200` machines |
| `GET` | `/api/machines/:machineId` | Bearer | `200` machine |
| `PATCH` | `/api/machines/:machineId` | Bearer | `200` machine |
| `DELETE` | `/api/machines/:machineId` | Bearer | `204` |
| `POST` | `/api/machines/:machineId/baselines` | Bearer | `201` baseline (multipart `audio`) |
| `GET` | `/api/machines/:machineId/baselines` | Bearer | `200` baselines |
| `POST` | `/api/machines/:machineId/inspections` | Bearer | `201` health card (multipart `audio`) |
| `GET` | `/api/machines/:machineId/inspections` | Bearer | `200` inspections |
| `POST` | `/api/machines/:machineId/inspections/:inspectionId/advisory/messages` | Bearer | `200` advisory reply |

`422` carries an actionable reason straight from the AI quality gate — an audio
clip that is too quiet, too short, or too clipped — or from the backend when a
machine has no baseline yet. Show its `error` text to the operator verbatim.

`KALIBRASI_KURANG` is **not** an error. It arrives as `201` with a full health
card whose `reason` explains what to re-record.

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
