# Security: Static Application Security Testing (SAST) dengan CodeQL

Dokumen ini mendokumentasikan aktivasi, cakupan analisis, temuan awal (*baseline scan*), keputusan & perbaikan keamanan, serta hasil *re-scan* Static Application Security Testing (SAST) menggunakan **GitHub CodeQL** pada repository ini.

---

## Ringkasan Konfigurasi & Cakupan Scan

- **Workflow**: [`.github/workflows/codeql.yml`](../.github/workflows/codeql.yml)
- **Trigger**: `push` (semua branch), `pull_request`, jadwal mingguan (`cron: '30 1 * * 1'`), dan `workflow_dispatch`.
- **Engine**: CodeQL Action `v4` (`CodeQL 2.27.6`), runner `ubuntu-24.04`.
- **Query Suite**: `security-extended` (mencakup seluruh query keamanan bawaan beserta query keamanan tambahan berjangkauan lebih luas).
- **Bukti Hasil Scan**:
  - Terunggah otomatis ke tab **GitHub Security → Code scanning** (`upload: always`).
  - Ringkasan tabel SARIF tercetak di **GitHub Actions → CodeQL SAST → Job Summary**.
  - File mentah SARIF tersimpan sebagai workflow artifact (`codeql-sarif-go`, `codeql-sarif-javascript-typescript`, `codeql-sarif-actions`, retensi 30 hari).
- **Bahasa yang Dianalisis**:
  1. **`go`** (`build-mode: manual`, `CGO_ENABLED=1 go build ./src/... ./web/...`) — mencakup seluruh kode backend, worker, API router, auth, access control, admin, explorer, migration, jobs, store, telemetry, dan embedded SPA server (86 artifact, 35 security rules).
  2. **`javascript-typescript`** (`build-mode: none`) — mencakup seluruh frontend SPA React/TypeScript (`frontend/src/**`), script legacy (`web/static/**`), test frontend/browser (`tests/unit/frontend/**`, `tests/browser/**`), dan script load test (`tests/k6/stress.js`).
  3. **`actions`** (`build-mode: none`) — mencakup seluruh definisi workflow CI/CD (`.github/workflows/ci.yml`, `.github/workflows/codeql.yml`).
  - *Catatan cakupan*: Aplikasi utama tidak menggunakan Python. File `tests/k6/{client,prepare,report}.py` hanyalah script helper lokal untuk stress testing k6 di luar runtime aplikasi sehingga tidak dimasukkan ke cakupan CodeQL aplikasi.

### Referensi Commit
| Tahap | Commit | Keterangan |
|---|---|---|
| Aktivasi CodeQL & Baseline Scan | `e4606b5`, `2c88b7b` | Menambahkan `.github/workflows/codeql.yml` dengan cakupan `go`, `javascript-typescript`, dan `actions` (`security-extended`). |
| Perbaikan Keamanan #1 | `06a0f7f` | Memisahkan `setSessionCookie` (`HttpOnly: true` eksplisit) dari `setCSRFCookie` (`HttpOnly: false`), serta menambahkan sanitasi `\r`/`\n` di `telemetry.Log` & `SanitizeValue` + unit test. |
| Perbaikan Keamanan #2 | `4ba2a06` | Menutup sisa *taint path* CodeQL pada cabang tipe numerik/boolean di `sanitizeLogArg` (`src/internal/telemetry/telemetry.go`). |
| Merge & Re-scan Akhir | `b21c1c5`, `b04466c` | Merge `feat/sast` ke `temp/integration` dan mengaktifkan trigger CodeQL di semua branch; hasil re-scan terkonfirmasi bersih untuk seluruh temuan yang diperbaiki. |

---

## 1. Analisis Go (`go`)

- **Cakupan kode**: Seluruh 19 package produksi pada `go.mod` (`src/api`, `src/app`, `src/cmd/server`, `src/cmd/worker`, `src/internal/{access,admin,apperror,auth,config,contracts,explorer,health,jobs,migration,models,sqlguard,store,telemetry}`, dan `web`), total 86 artifact dengan 35 rules `security-extended`.

### 1.1 Test Awal (Baseline Scan — Commit `2c88b7b`)

#### CodeQL SARIF Summary (4 finding(s))

| Rule | Level | Security Severity | Location | Message |
|---|---|---|---|---|
| `go/sql-injection` | `error` | `8.8` | `src/internal/explorer/explorer_repository.go:467` | This query depends on a [user-provided value](1). This query depends on a [user-provided value](2). |
| `go/sql-injection` | `error` | `8.8` | `src/internal/explorer/explorer_repository.go:389` | This query depends on a [user-provided value](1). This query depends on a [user-provided value](2). |
| `go/cookie-httponly-not-set` | `warning` | `5.0` | `src/internal/auth/auth_handler.go:23` | Sensitive cookie [authHandler](1) does not set HttpOnly attribute to true. (7 related locations) |
| `go/log-injection` | `error` | `6.1` | `src/internal/telemetry/telemetry.go:328` | This log entry depends on a [user-provided value](1)..(22). |

### 1.2 Detail Setiap Temuan (Severity, Potensi Dampak & Keputusan)

1. **`go/sql-injection` (CWE-089) — `src/internal/explorer/explorer_repository.go:467` (`ExplainQuery`)**
   - **Severity**: `error` (`security-severity: 8.8`, High)
   - **Aliran data**: Body JSON `POST /api/v1/databases/{dbId}/queries/explain` (`explorer_handler.go:206-208`) → `explorer_service.go:49` → `explorer_repository.go:467` (`tx.QueryContext(qCtx, explainSQL)`).
   - **Potensi dampak**: Jika tanpa kontrol, input SQL arbitrer dapat mengeksekusi perintah destruktif (DDL/DML), multi-statement injection, atau membebani DBMS.
   - **Klasifikasi & Keputusan**: **Risiko Diterima (*Accepted Risk* / Intentional by Design) — Tidak Diperbaiki.**

2. **`go/sql-injection` (CWE-089) — `src/internal/explorer/explorer_repository.go:389` (`RunQuery`)**
   - **Severity**: `error` (`security-severity: 8.8`, High)
   - **Aliran data**: Body JSON `POST /api/v1/databases/{dbId}/queries` (`explorer_handler.go:186-188`) → `explorer_service.go:157` → `explorer_repository.go:389` (`tx.QueryContext(qCtx, input.SQL)`).
   - **Potensi dampak**: Jika tanpa kontrol, input SQL arbitrer dapat memodifikasi/menghapus data atau membaca di luar batasan hak akses.
   - **Klasifikasi & Keputusan**: **Risiko Diterima (*Accepted Risk* / Intentional by Design) — Tidak Diperbaiki.**

3. **`go/cookie-httponly-not-set` (CWE-1004) — `src/internal/auth/auth_handler.go:23-31` (`setCookie`)**
   - **Severity**: `warning` (`security-severity: 5.0`, Medium)
   - **Aliran data**: `h.Service.Policy.CookieName` (cookie sesi `sid`) mengalir ke helper tunggal `setCookie(..., httpOnly bool, ...)` yang juga dipakai dengan `httpOnly=false` untuk cookie `csrf_token`.
   - **Potensi dampak**: Jika cookie sesi diterbitkan tanpa flag `HttpOnly: true`, skrip sisi klien dapat mencuri token sesi melalui `document.cookie` saat terjadi XSS (*session hijacking*).
   - **Klasifikasi & Keputusan**: **Temuan Valid (Hardening Struktural) — Diperbaiki.**

4. **`go/log-injection` (CWE-117) — `src/internal/telemetry/telemetry.go:328` (`Log` → `slog.Log`)**
   - **Severity**: `error` (`security-severity: 6.1`, Medium)
   - **Aliran data**: 22 sumber input HTTP user (`r.URL.Path`, `dbID` di `access_middleware.go` dan `telemetry.go:405`, body JSON `username`/nama resource/query di `auth_handler.go`, `admin_handler.go`, `explorer_handler.go`, `migration_handler.go`) diteruskan ke `slog.Log` tanpa penghapusan `\r`/`\n`.
   - **Potensi dampak**: *Log forging* — penyerang dapat menyisipkan karakter baris baru (`\r`/`\n`) untuk memalsukan entri log atau mengganggu parser/pipeline log (termasuk saat diteruskan melalui OTLP log bridge).
   - **Klasifikasi & Keputusan**: **Temuan Valid — Diperbaiki.**

### 1.3 Langkah yang Diambil

| Rule | Location | Langkah / Alasan |
|---|---|---|
| `go/sql-injection` | `src/internal/explorer/explorer_repository.go:467` | **Tidak diperbaiki (Risiko Diterima / Intentional by Design).** Fitur inti Database Explorer memang menjalankan `EXPLAIN` atas query SQL yang ditulis pengguna, sehingga struktur SQL tidak dapat dijadikan *parameterized query* statis. Risiko dimitigasi berlapis (*defense-in-depth*): (1) wajib autentikasi sesi + RBAC minimal `VIEW_ONLY` + validasi CSRF (`src/api/routes.go:143`); (2) validasi AST ketat via parser PostgreSQL `pg_query_go/v5` (`ValidateReadOnlySQL`: tepat 1 statement `SELECT`, larang `SELECT INTO`, larang `FOR UPDATE/SHARE`, larang data-modifying CTE); (3) dieksekusi dengan kredensial role `reader` terpisah di dalam transaksi `ReadOnly: true` dengan `QueryTimeout`; dan (4) transaksi **selalu di-`Rollback()`**. |
| `go/sql-injection` | `src/internal/explorer/explorer_repository.go:389` | **Tidak diperbaiki (Risiko Diterima / Intentional by Design).** Fitur inti SQL Query Runner memang mengeksekusi query `SELECT` kustom pengguna. Risiko dimitigasi berlapis: (1) autentikasi sesi + RBAC `VIEW_ONLY` + CSRF (`src/api/routes.go:142`); (2) validasi AST ketat via `ValidateReadOnlySQL` (PostgreSQL) / `ValidateMySQLQuery` (MySQL) yang membatasi tepat 1 statement `SELECT` read-only; (3) koneksi role `reader` terpisah; (4) transaksi `ReadOnly: true` dengan `QueryTimeout`, batas `MaxRows`, dan selalu di-`Rollback()`. |
| `go/cookie-httponly-not-set` | `src/internal/auth/auth_handler.go:23` | **Diperbaiki (commit `06a0f7f`).** Memecah helper `setCookie(..., httpOnly bool, ...)` menjadi dua fungsi eksplisit di `src/internal/auth/auth_handler.go` dan `src/internal/auth/auth_middleware.go`: `setSessionCookie` yang mengunci `HttpOnly: true` secara literal (mustahil salah konfigurasi untuk cookie sesi `sid`), dan `setCSRFCookie` (`Name: "csrf_token"`, `HttpOnly: false`) khusus pola *double-submit CSRF* SPA. Diverifikasi dengan unit test baru `TestLoginHandlerCookieFlags` di `tests/unit/auth/auth_login_test.go`. |
| `go/log-injection` | `src/internal/telemetry/telemetry.go:328` | **Diperbaiki (commit `06a0f7f` & `4ba2a06`).** Menambahkan `stripLogNewlines` (`\r` → `""`, `\n` → `" "`) dan `sanitizeLogArg` di `src/internal/telemetry/telemetry.go` untuk menyaring `event`, seluruh atribut log, dan field korelasi ke slice baru sebelum `slog.Log` (serta pada `SanitizeValue`). Pada `4ba2a06`, jalur *taint* tersisa pada cabang `int`/`int64`/`float64`/`bool` ditutup dengan round-trip `strconv` + literal boolean agar tidak ada cabang yang mengembalikan nilai mentah tanpa sanitasi sembari mempertahankan tipe JSON. Diverifikasi dengan unit test baru `TestLogSanitizesNewlinesAgainstLogInjection` di `tests/unit/telemetry/telemetry_test.go`. |

### 1.4 Hasil Tes Akhir (Re-scan — Commit `b04466c`)

#### CodeQL SARIF Summary (2 finding(s))

| Rule | Level | Security Severity | Location | Message |
|---|---|---|---|---|
| `go/sql-injection` | `error` | `8.8` | `src/internal/explorer/explorer_repository.go:467` | This query depends on a [user-provided value](1). This query depends on a [user-provided value](2). |
| `go/sql-injection` | `error` | `8.8` | `src/internal/explorer/explorer_repository.go:389` | This query depends on a [user-provided value](1). This query depends on a [user-provided value](2). |

- **Ringkasan hasil Go**: Temuan berkurang dari **4 → 2**. Kedua temuan yang diperbaiki (`go/cookie-httponly-not-set` dan `go/log-injection`) **terkonfirmasi hilang (0 alert)** pada hasil scan ulang. Dua temuan yang tersisa merupakan fitur inti query SQL yang sudah didokumentasikan sebagai risiko diterima dengan mitigasi berlapis.

---

## 2. Analisis JavaScript / TypeScript (`javascript-typescript`)

- **Cakupan kode**: Seluruh source SPA React/TypeScript (`frontend/src/**`, `frontend/vite.config.ts`), aset JS legacy (`web/static/api.mjs`, `web/static/app.js`), unit/browser test JS (`tests/unit/frontend/**`, `tests/browser/**`), dan script k6 (`tests/k6/stress.js`).

### 2.1 Test Awal (Baseline Scan — Commit `2c88b7b`)

#### CodeQL SARIF Summary (0 finding(s))

No findings reported in generated SARIF files.

### 2.2 Langkah yang Diambil

| Rule | Location | Langkah / Alasan |
|---|---|---|
| *(Tidak ada temuan)* | `frontend/src/**`, `web/static/**`, `tests/**/*.js`, `tests/**/*.mjs` | **Tidak diperlukan perbaikan dari temuan CodeQL.** Seluruh kode frontend SPA tidak menggunakan sink DOM berbahaya (`dangerouslySetInnerHTML`, `innerHTML`, `eval`, `document.write`), menggunakan `credentials: 'same-origin'` + header `X-CSRF-Token`, serta dilindungi CSP ketat (`script-src 'self'`) dari `web/web.go`. Cakupan analisis dan batasannya tetap didokumentasikan karena hasil bersih tidak menjamin aplikasi pasti bebas kerentanan. |

### 2.3 Hasil Tes Akhir (Re-scan — Commit `b04466c`)

#### CodeQL SARIF Summary (0 finding(s))

No findings reported in generated SARIF files.

---

## 3. Analisis GitHub Actions (`actions`)

- **Cakupan kode**: Seluruh workflow GitHub Actions pada repository (`.github/workflows/ci.yml` dan `.github/workflows/codeql.yml`).

### 3.1 Test Awal (Baseline Scan — Commit `2c88b7b`)

#### CodeQL SARIF Summary (0 finding(s))

No findings reported in generated SARIF files.

### 3.2 Langkah yang Diambil

| Rule | Location | Langkah / Alasan |
|---|---|---|
| *(Tidak ada temuan)* | `.github/workflows/ci.yml`, `.github/workflows/codeql.yml` | **Tidak diperlukan perbaikan dari temuan CodeQL.** Workflow sudah menerapkan prinsip *least privilege* (`permissions: contents: read` di level workflow dan izin minimal per job), `persist-credentials: false` pada `actions/checkout`, SHA pinning pada action pihak ketiga/eksternal, serta bebas interpolasi ekspresi tidak tepercaya (`${{ github.event.* }}`) di dalam blok `run:`. |

### 3.3 Hasil Tes Akhir (Re-scan — Commit `b04466c`)

#### CodeQL SARIF Summary (0 finding(s))

No findings reported in generated SARIF files.

---

## 4. Keterbatasan SAST — Hasil Bersih Tidak Berarti Aplikasi Pasti Aman

Meskipun hasil akhir menunjukkan **0 temuan** pada `javascript-typescript` dan `actions`, serta **0 temuan valid tersisa** pada `go` (hanya 2 temuan `go/sql-injection` *intentional by design*), **hasil scan CodeQL yang bersih tidak menjamin aplikasi pasti aman**, karena:

1. **Analisis statis tanpa eksekusi runtime**: SAST tidak menjalankan aplikasi seperti DAST (*Dynamic Application Security Testing*), sehingga tidak mendeteksi kerentanan yang baru muncul saat runtime, interaksi antar-layanan, atau konfigurasi infrastruktur (misalnya TLS/firewall/reverse proxy).
2. **Cacat *business logic* & *broken access control* kompleks**: CodeQL tidak dapat memverifikasi aturan domain spesifik aplikasi (seperti urutan approval multi-group leader, isolasi lock lintas worker, atau privilege escalation berbasis alur bisnis). Hal ini tetap harus diuji melalui *automated unit/integration testing* dan *manual code review*.
3. **Keterbatasan cakupan query (*false negatives*)**: Hanya pola yang dimodelkan pada query suite `security-extended` yang terdeteksi. Potensi risiko lain (seperti kerentanan pada library pihak ketiga/CVE, validasi host/DSN target database admin, atau celah *edge-case* parser) tetap memerlukan *Software Composition Analysis* (SCA / Dependabot), *secret scanning*, dan review manual berkala.
4. **Cakupan file di luar runtime**: File konfigurasi environment lokal, container runtime, serta script helper pengujian lokal (`tests/k6/*.py`) tidak tercakup oleh analisis aplikasi di atas.

---

## 5. Cara Memverifikasi & Menjalankan Ulang Scan

1. **Tab Security GitHub**: Buka repository di GitHub → **Security → Code scanning** untuk melihat status alert CodeQL terbaru per bahasa (`/language:go`, `/language:javascript-typescript`, `/language:actions`).
2. **Tab Actions GitHub**: Buka **Actions → CodeQL SAST** → pilih run terbaru untuk melihat tabel **CodeQL SARIF Summary** pada Job Summary serta mengunduh artifact `codeql-sarif-go`, `codeql-sarif-javascript-typescript`, dan `codeql-sarif-actions`.
3. **Menjalankan ulang secara manual**: Pada halaman **Actions → CodeQL SAST**, klik **Run workflow** dan pilih branch yang ingin dianalisis.
