<img width="400" alt="Image" src="https://static.wikia.nocookie.net/vocaloid/images/6/69/Triple_baka.png/revision/latest?cb=20131110004348"/>

# Tugas Besar Milestone 1 IF3110 Pengembangan Aplikasi Web 2026

Database Management Platform

## Identitas Pembuat

| Nama | NIM |
|--------------------------------|----------|
| Muhammad Fatih Irkham Mauludi | 13524004 |
| Anindya Naufal Pinashtika | 13524013 |
| Syaqina Octavia Rizha | 13524088 |

## Deskripsi

Database Management Platform adalah fondasi platform pengelolaan basis data PostgreSQL yang mengatur akses, membantu pengguna mengeksplorasi database, dan mencatat perubahan schema secara terkontrol.

Fitur utama yang diimplementasikan pada milestone ini:

- **Access Control** — autentikasi username/password dengan password hashing (bcrypt), session management lengkap (opaque identifier, rotasi berkala, idle timeout, absolute expiration, grace period, CSRF), dan otorisasi berbasis **group** dengan dua tingkat akses: `VIEW_ONLY` (melihat schema/data/riwayat migration) dan `VIEW_EDIT` (seluruh akses `VIEW_ONLY` + membuat/mengubah/menjalankan migration).
- **Platform Admin** — mengelola user, group, koneksi database, dan pemberian akses. Platform Admin bukan tingkatan akses terhadap suatu database; akses database diberikan melalui group, dan akses efektif dihitung sebagai tingkat tertinggi (`VIEW_EDIT` menang atas `VIEW_ONLY`) bila user tergabung di beberapa group.
- **Database Viewer** — menjelajahi schema, table, column, constraint, index, dan data dari database target.
- **Migration** — menyusun, mengubah, dan mengeksekusi perubahan schema dalam bentuk SQL yang dijalankan secara transaksional (commit bila berhasil, rollback bila gagal) dengan riwayat linear per database.

## Requirements

- **Go 1.27+**
- **Docker** & **Docker Compose** (PostgreSQL 17 + backend dijalankan dalam container)
- **Node.js 18+** & **npm** (hanya untuk test frontend/browser, opsional)

Referensi instalasi:

- **Go**: [Go Downloads](https://go.dev/dl/)
- **Docker**: [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- **Node.js + npm**: [Node.js Downloads](https://nodejs.org/en/download)

## Cara Setup dan Menjalankan Program

Jalankan dari direktori **root** project.

### 1. Siapkan file `.env` (sekali saja)

```bash
cp .env.example .env
```

Lalu isi `DB_CREDENTIAL_ENCRYPTION_KEY` dengan key base64 32-byte. Generate dengan:

```bash
openssl rand -base64 32
```

Contoh isi `.env`:

```env
DB_CREDENTIAL_ENCRYPTION_KEY=AbC123...   # hasil openssl rand -base64 32
```

### 2. Jalankan seluruh stack

```bash
docker compose up -d --build
```

Perintah ini menjalankan: platform database (PostgreSQL), tiga database target, dan backend.

Jika platform database sudah menggunakan volume lama, jalankan upgrade schema satu kali setelah menarik versi terbaru:

```bash
make migrate-platform
```

Migration ini menambahkan kolom `engine` yang dibutuhkan metadata koneksi. Init script Compose menerapkannya otomatis pada volume baru; Compose tidak menjalankan ulang init script pada volume yang sudah ada.

### 3. Akses aplikasi

- **WEB**: <http://localhost:28080>
- **Health check**: <http://localhost:28080/health>

```bash
curl http://localhost:28080/health
# {"status":"ok"}
```

### Akun default (development seed)

| Role | Username | Password |
|------|----------|----------|
| Platform Admin | `admin` | `AdminDemo-2026!` |
| User | `user1` | `user1-12345` |
| User | `user2` | `user2-12345` |
| User | `user3` | `user3-12345` |
| User | `user4` | `user4-12345` |
| User | `user5` | `user5-12345` |

> Seed hanya di-apply saat pertama kali volume PostgreSQL dibuat. Untuk reset password admin, jalankan `db/platform/000004_reset_dev_admin.sql`.

## Menjalankan Test

```bash
make check              # go vet + syntax check frontend
make test               # unit test (Go + frontend) tanpa butuh PostgreSQL
make test-db-up         # starts the disposable PostgreSQL test service on port 15433
make test-integration   # integration test (butuh link TEST_DATABASE_URL ke database bernamae "*_test")
make test-browser       # also requires TEST_DATABASE_URL and Playwright
make build              # build binary server ke tests/artifacts/
```

## Struktur Proyek

```
IF3110_DB-Management-Platform
├── README.md
├── Makefile
├── docker-compose.yml
├── go.mod
├── .env.example
├── .github/
│   └── workflows/
│       └── ci.yml
├── db/
│   ├── platform/
│   │   ├── 000001_create_platform_schema.sql
│   │   ├── 000002_dev_admin.sql
│   │   ├── 000003_dev_user.sql
│   │   └── 000004_reset_dev_admin.sql
│   └── target/
│       └── 01-init-target-db.sql
├── src/
│   ├── Dockerfile
│   ├── api/
│   │   ├── routes.go          # registrasi seluruh route + middleware
│   │   ├── router.go
│   │   ├── types.go           # DTO request/response
│   │   └── utils.go
│   ├── app/
│   │   └── router.go          # wiring dependency antar domain
│   ├── cmd/
│   │   └── server/
│   │       └── main.go
│   └── internal/
│       ├── auth/              # autentikasi + session management
│       │   ├── auth_handler.go
│       │   ├── auth_middleware.go
│       │   ├── auth_password.go
│       │   ├── auth_repository.go
│       │   ├── auth_routes.go
│       │   ├── auth_service.go
│       │   ├── auth_token.go
│       │   ├── auth_types.go
│       │   └── auth_utils.go
│       ├── access/            # otorisasi: akses efektif + middleware guard
│       │   ├── access_middleware.go
│       │   ├── access_repository.go
│       │   └── access_service.go
│       ├── admin/             # pengelolaan user/group/database/grant
│       │   ├── admin_handler.go
│       │   ├── admin_repository.go
│       │   └── admin_service.go
│       ├── explorer/          # viewer schema/table/data + query read-only
│       │   ├── explorer_handler.go
│       │   ├── explorer_query.go
│       │   ├── explorer_repository.go
│       │   └── explorer_service.go
│       ├── migration/         # migration: draft, eksekusi transaksional
│       │   ├── migration_handler.go
│       │   ├── migration_repository.go
│       │   ├── migration_runner.go
│       │   └── migration_service.go
│       ├── store/             # koneksi platform & target + enkripsi kredensial
│       │   ├── credentials.go
│       │   └── postgres.go
│       ├── models/
│       │   └── platform.go    # model GORM (7 tabel metadata)
│       ├── config/
│       │   └── config.go
│       ├── apperror/
│       │   └── errors.go
│       └── health/
│           └── health_handler.go
├── web/                       # frontend Go template (non-SPA)
│   ├── web.go
│   ├── static/
│   │   ├── api.mjs
│   │   ├── app.css
│   │   ├── app.js
│   │   └── favicon.ico
│   └── templates/
│       ├── admin.html
│       ├── base.html
│       ├── explorer.html
│       └── migrations.html
└── tests/
    ├── go.mod
    ├── compose.yml
    ├── package.json
    ├── browser/               # test E2E Playwright
    ├── integration/
    │   ├── auth/
    │   ├── admin/
    │   └── web/
    ├── internal/
    │   └── testutil/
    └── unit/
        ├── access/
        ├── admin/
        ├── auth/
        ├── config/
        ├── frontend/
        └── telemetry/
```

## Activity Logging, Tracing & Bonus Observability

- Aplikasi mengeluarkan structured JSON logs (`log/slog`) ke `stdout` dengan redaction otomatis untuk credential/token/session dan korelasi `trace_id`/`span_id` OpenTelemetry lintas `API -> Queue -> Worker -> Lock -> Migration/Batch/SQL` serta query read-only Explorer.
- Endpoint metrik Prometheus tersedia di `GET /metrics` (<http://localhost:6767/metrics>).

### Cara Menjalankan

Pastikan `.env` sudah siap (jalankan `./scripts/setup.sh` sekali jika belum). Untuk menjalankan stack lengkap dengan **OpenTelemetry Collector + Tempo + Loki + Prometheus + Grafana** (dashboard auto-provisioned):

```bash
docker compose -f docker-compose.yml -f docker-compose.observability.yml up -d --build
```

Untuk menyertakan container `worker` terpisah (profile `jobs`, mengekspor metrik worker di internal `worker:9091/metrics`):

```bash
docker compose --profile jobs -f docker-compose.yml -f docker-compose.observability.yml up -d --build
```

Untuk melihat structured JSON logs secara langsung:

```bash
docker compose logs -f backend
# jika menjalankan worker terpisah:
docker compose --profile jobs logs -f worker
```

Untuk menghentikan seluruh stack observability:

```bash
docker compose --profile jobs -f docker-compose.yml -f docker-compose.observability.yml down
```

### Link & Path Layanan Observability

| Layanan | URL / Path | Keterangan |
|---|---|---|
| **Web & API** | <http://localhost:6767> | Aplikasi utama (health check: <http://localhost:6767/health>) |
| **Prometheus Metrics (Backend)** | <http://localhost:6767/metrics> | Endpoint metrik format Prometheus (`platform-api`) |
| **Prometheus UI** | <http://localhost:9090> | Query & graph: <http://localhost:9090/graph> · Status scrape targets: <http://localhost:9090/targets> |
| **Grafana** | <http://localhost:3000> | Anonymous Admin aktif, atau login `admin` / `admin` |
| **Grafana Dashboard (Auto-provisioned)** | <http://localhost:3000/d/db-platform-observability> | *DB Management Platform — Queue, Migration & DB Pool* |
| **Grafana Explore (Loki & Tempo)** | <http://localhost:3000/explore> | Korelasi log ↔ trace via `TraceID` (Loki → Tempo & Tempo → Loki) |
| **OpenTelemetry Collector (OTLP)** | `http://localhost:4318` (HTTP) · `localhost:4317` (gRPC) | Penerima OTLP traces & logs |
| **Tempo / Loki API** | <http://localhost:3200> (Tempo) · <http://localhost:3100> (Loki) | Backend storage traces & logs |

Lihat detail arsitektur, contoh output log/span/metrik, dan cara korelasi log-trace di [docs/observability.md](docs/observability.md).

## Stress Testing dengan k6

Stress test dilakukan dengan menjalankan 
```bash
./scripts/setup.sh
```
dari root project secara otomatis, jikalau ingin melakukan stress test terpisah dapat dilakukan dengan menjalankan
```bash
./scripts/stress.sh
``` 
dimana script tersebut basically merupakan script untuk stress testing, dimana awalnya akan
membuat project Compose **if3110-stress** dengan platform DB, dua PostgreSQL
target, backend, worker, k6, script Python, dan OpenSSL (tidak membuka port/queue, jadi hal tsb merupakan pengecualian). Stress test dilakukan menjadi 3 tahap yakni:

```bash
./scripts/stress.sh smoke  # verifikasi pendek API -> approval -> queue -> worker -> done
./scripts/stress.sh run    # stress test (kasus db sama & berbeda)
./scripts/stress.sh down   # stop stress test
```

### Cara kerja script dan metrik

- `tests/k6/prepare.py`: merupakan script e2e python yang melakukan semua bootstrap login, buat group/leader/grant, register db target, queue, migration, dan approval. Dimana pada proses ini diasumsikan happy path (semua proses dijalankan sesuai ekspetasi).
- `tests/k6/stress.js`: merupakan script untuk menambahkan Virtual User (VU). Setiap VU login dengan cookie jar sendiri, mengakses schema/data, dan mengirim migration ID unik yang belum pernah digunakan serta melakukan polling pada setiap job (`SUCCEEDED`, `FAILED`/`BLOCKED`, atau timeout). Karena ini merupakan stress testing, purpose dari VU ini adalah mengetes seberapa banyak VU yang web dapat handle (dimana VU akan perlahan bertambah) sebelum mencapai batas failure pada p95 response time yang dimana nantinya akan diukur.
- `tests/k6/report.py`: merupakan script untuk menghitung dan memberikan summary dari seluruh VU job di web, hal-hal tersebut berupa konsiderasi job yang mungkin timeout/proses tidak berhasil (tidak tersebumit)

Pada inti dari stress test seniri, tiap VU memiliki workload default setiap job berupa INSERT 1.000 row dengan payload 128 karakter, ditambah `pg_sleep(2)` dalam transaksi untuk memberi durasi eksekusi sintetis yang terkontrol dan memperlihatkan antrean. Sedangkan untuk SQL down nya akan menghapus row job tersebut dan tes diasumsikan tidak menjalankan rollblack. Untuk Sleep, dilakukan pengukuran berupa data perubahan melalui `JOB_DELAY` (termasuk 0 untuk mengukur INSERT tanpa delay).

Setelah stress test berhasil, hasil disimpan di `tests/artifacts/k6/<timestamp>-<same|different>/`:
`fixture.json`, `raw.json` (sampel k6 bertimestamp), `summary.json`, `jobs.json`,
`report.json`, `report.md`, `application.log`, dan `environment.txt`.


### Profil beban dan threshold

Berikut merupakan konfigurasi beban yang dilakukan pada mekanisme stress testing

| Fase | Durasi plateau | Target API baca | Target pengiriman migration |
|---|---:|---:|---:|
| Normal | 60 s | 10 iterasi/s | 12 job/menit |
| 2× normal | 60 s | 20 iterasi/s | 24 job/menit |
| 4× normal | 60 s | 40 iterasi/s | 48 job/menit |
| Recovery | 60 s | 10 iterasi/s | 12 job/menit |

Ditetapkan juga bahwa untuk threshold k6 memakai `http_req_duration{kind:api}: p(95)<500` ms dan `http_req_failed{kind:api}: rate<0.01` (di bawah 1%)

### Hasil dan analisis

Untuk menjalankan ulang dan meninjau laporan dapat dilakukan dengan melakukan:

```bash
./scripts/stress.sh run
cat tests/artifacts/k6/*/report.md
```

Dimana pada stress test yang kami lakukan, pengujian memakai:
- plateau **20 detik**, 
- ramp **5 detik**, 
- API baca normal 10 iterasi/s, 
- migration normal 12 job/menit, 
- dan worker concurrency 4
- Setiap job INSERT 1.000 row + delay sintetis 2 detik

(kurang lebih aturan dan konfigurasi sama dengan default/apa yang dibahas sebelumnya)

Dengan mesin 
- OS: Ubuntu 24.04.3 LTS, 
- CPU: 20 vCPU
- RAM: ~8GiB 

Beriku hasil stress testingnya

**Database sama**
| Fase | API p50 ms | API p95 ms | req/s aktual | Error % | Queue p95 ms | Durasi job p95 ms | Job selesai | job/s | Backlog akhir |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| normal | 9.75 | 16.39 | 8.95 | 0.00 | 171.12 | 2041.66 | 3 | 0.150 | 0 |
| ramp_2x | 10.33 | 14.99 | 11.60 | 0.00 | 183.44 | 2027.63 | 1 | 0.200 | 0 |
| 2x | 8.88 | 40.35 | 18.10 | 0.00 | 500.55 | 4098.20 | 6 | 0.300 | 1 |
| ramp_4x | 46.36 | 1115.29 | 14.40 | 0.00 | 418.35 | 3048.36 | 2 | 0.400 | 1 |
| 4x | 10.39 | 68.13 | 38.45 | 0.00 | 13852.87 | 2944.40 | 9 | 0.450 | 7 |
| ramp_down | 8.53 | 14.20 | 40.80 | 0.00 | 17446.98 | 2033.09 | 2 | 0.400 | 9 |
| recovery | 8.58 | 14.96 | 12.95 | 0.00 | 18676.84 | 2038.83 | 9 | 0.450 | 5 |
| drain | — | — | 0.00 | — | 6681.65 | 2033.79 | 6 | 0.376 | 0 |

Dapat dilihat dari stress test bahwa 
- p95 API agregat **57.99 ms**, 
- error rate **0%**

**Database berbeda**
| Fase | API p50 ms | API p95 ms | req/s aktual | Error % | Queue p95 ms | Durasi job p95 ms | Job selesai | job/s | Backlog akhir |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| normal | 10.40 | 16.70 | 10.10 | 0.00 | 60.06 | 2043.23 | 3 | 0.150 | 0 |
| ramp_2x | 8.13 | 20.21 | 5.80 | 0.00 | 77.09 | 2059.57 | 0 | 0.000 | 1 |
| 2x | 9.05 | 16.77 | 20.05 | 0.00 | 309.72 | 2045.53 | 8 | 0.400 | 1 |
| ramp_4x | 10.84 | 14.59 | 22.40 | 0.00 | 232.24 | 2037.79 | 2 | 0.400 | 1 |
| 4x | 10.37 | 15.28 | 34.65 | 0.00 | 1494.82 | 2967.89 | 13 | 0.650 | 2 |
| ramp_down | 10.78 | 15.31 | 40.80 | 0.00 | 611.73 | 2038.29 | 4 | 0.800 | 2 |
| recovery | 10.78 | 15.37 | 13.45 | 0.00 | 623.32 | 2040.74 | 6 | 0.300 | 1 |
| drain | 10.64 | 18.77 | 5.91 | 0.00 | 581.18 | 2041.14 | 3 | 0.225 | 0 |

Dapat dilihat dari stress test bahwa 
- p95 API agregat **15.93 ms**
- error rate **0%**
