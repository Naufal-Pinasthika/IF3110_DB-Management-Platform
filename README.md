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
        └── frontend/
```
