<img width="400" alt="Image" src="https://static.wikia.nocookie.net/vocaloid/images/6/69/Triple_baka.png/revision/latest?cb=20131110004348"/>

# Tugas Besar Milestone 2 IF3110 Pengembangan Aplikasi Web 2026

Database Management Platform

## Identitas Pembuat

| Nama | NIM |
|---|---|
| Muhammad Fatih Irkham Mauludi | 13524004 |
| Anindya Naufal Pinashtika | 13524013 |
| Syaqina Octavia Rizha | 13524088 |

## Deskripsi

Platform pengelolaan database PostgreSQL dan MySQL dengan frontend React SPA dan backend Go, dimana Fitur utama hingga Milestone 2:

- Autentikasi berbasis session dan akses database melalui group (`VIEW_ONLY` / `VIEW_EDIT`).
- Administrasi user, group, leader, koneksi database, dan hak akses.
- Explorer schema/data serta query read-only.
- Migration SQL UP/DOWN, approval leader, rollback/forward, dan riwayat eksekusi.
- Eksekusi asynchronous melalui queue dan worker, termasuk migration per batch dengan checkpoint.
- Structured logging, OpenTelemetry tracing, dan metrik Prometheus.

## Setup

Butuh **Docker dengan Compose v2**, **OpenSSL**, dan Bash. Go, Node.js, dan psql lokal tidak diperlukan untuk menjalankan stack.

Dari root project:

```bash
./scripts/setup.sh
```

Script menyiapkan `.env` dan encryption key jika belum ada, membangun frontend/backend/worker, menerapkan upgrade schema platform, serta menjalankan database, web, dan worker migration. Eksekusi ulang mempertahankan `.env`, key, dan data. Worker juga memproses antrean job yang tersimpan.

- **Web:** <http://localhost:6767>
- **Health:** <http://localhost:6767/health>

```bash
docker compose logs -f backend worker
# Hentikan stack, data tetap tersimpan:
docker compose --profile jobs down
```

Untuk menghapus seluruh data database project dan membangun ulang, gunakan `./scripts/setup.sh --reset`.

### Akun development

| Role | Username | Password |
|---|---|---|
| Platform Admin | `admin` | `AdminDemo-2026!` |
| User | `user1` sampai `user5` | Sesuai username: `user1-12345` sampai `user5-12345` |

Akun seed dibuat saat volume platform pertama kali diinisialisasi. Reset password admin tersedia di `db/platform/000004_reset_dev_admin.sql`.

### Koneksi database lokal

| Service Compose | Host dari container backend/worker | Port dari host | Database |
|---|---|---|---|
| `postgres` | `postgres:5432` | `56732` | `platform_db` (metadata) |
| `target-db-1` | `target-db-1:5432` | `56733` | `target_db` |
| `target-db-2` | `target-db-2:5432` | `56734` | `target_db` |
| `target-db-3` | `target-db-3:5432` | `56735` | `target_db` |
| `target-mysql` | `target-mysql:3306` | `56736` | `target_db` |

Saat mendaftarkan koneksi target lewat UI, gunakan host service dan port internal di atas. Dari host, gunakan `127.0.0.1` dan port host. User target: `target_db_user`; password PostgreSQL: `target_db_password_12345`, MySQL: `target_db_password_12345`. Metadata platform memakai `admin` / `root`; detail konfigurasi ada di [docker-compose.yml](docker-compose.yml).

## Pengujian

Untuk pengembangan/test lokal, siapkan Go sesuai `go.mod`, Node.js/npm, dan dependency frontend/test:

```bash
npm --prefix frontend ci
npm --prefix tests ci
make check
make test
make build
```

Integration/browser test memakai database terpisah:

```bash
make test-db-up
export TEST_DATABASE_URL='postgres://platform_test:platform_test@127.0.0.1:15433/platform_ci_test?sslmode=disable'
make test-integration
make browser-install
make test-browser
```

### Dataset batch migration

```bash
./scripts/generate-batch.sh
# Ulangi setelah job tes sebelumnya selesai; mengganti tabel batch:
./scripts/generate-batch.sh --reset
```

Script membuat tabel `batch` dengan 500.000 baris pada `target-db-1`, mengukur UPDATE seluruh baris, lalu me-rollback baseline. Gunakan `ROWS=1000000` untuk memperbesar dataset dan `TARGET_SERVICE=target-db-2` untuk mengganti target. Catat baseline **>1 detik**, lalu uji dataset yang sama melalui mode Batch di UI dengan cursor `id` dan batch size `10000`. SQL UP/DOWN harus merujuk tabel `batch`.

## Struktur Proyek

| Path | Isi |
|---|---|
| `frontend/` | React SPA |
| `src/` | API Go, domain aplikasi, server dan worker |
| `web/` | Asset frontend yang di-embed ke server |
| `db/` | Schema platform dan inisialisasi target |
| `scripts/` | Setup, seed batch, dan stress test |
| `tests/` | Unit, integration, browser, dan k6 |
| `docs/` | Dokumentasi observability dan SAST |

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

## Security: SAST dengan CodeQL

Static Application Security Testing (SAST) diaktifkan menggunakan **GitHub CodeQL** (`security-extended`) melalui workflow [`.github/workflows/codeql.yml`](.github/workflows/codeql.yml) untuk cakupan bahasa **`go`**, **`javascript-typescript`**, dan **`actions`**.

- **Hasil Baseline Scan** (`2c88b7b`): `go` = 4 temuan (`2× go/sql-injection`, `1× go/cookie-httponly-not-set`, `1× go/log-injection`); `javascript-typescript` = 0 temuan; `actions` = 0 temuan.
- **Perbaikan**: `go/cookie-httponly-not-set` dan `go/log-injection` diperbaiki pada commit `06a0f7f` & `4ba2a06`, sedangkan `2× go/sql-injection` pada Database Explorer (`RunQuery` & `ExplainQuery`) didokumentasikan sebagai *risiko diterima / intentional by design* dengan mitigasi berlapis (RBAC + CSRF + AST SQL validator + role `reader` + transaksi `ReadOnly: true` yang selalu di-rollback).
- **Hasil Re-scan Akhir** (`b04466c`): `go` = 2 temuan (*intentional by design*), `javascript-typescript` = 0 temuan, `actions` = 0 temuan.

Lihat dokumentasi lengkap (tabel SARIF test awal, langkah/alasan tiap temuan, detail dampak & keputusan, hasil re-scan, serta catatan keterbatasan SAST) di [docs/sast.md](docs/sast.md).
