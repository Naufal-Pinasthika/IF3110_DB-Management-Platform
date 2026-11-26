# Activity Logging, OpenTelemetry Tracing & Observability

Dokumen ini menjelaskan implementasi **Activity Logging**, **OpenTelemetry Distributed Tracing**, dan **Bonus Observability** (OpenTelemetry Collector, Tempo, Loki, Prometheus, dan Grafana).

---

## 1. Arsitektur

```text
HTTP Request (API)
  ├─ telemetry.HTTP (root span: http.request, X-Request-ID)
  ├─ sessionMiddleware (attach actor ke context)
  ├─ auth / access / admin / explorer (structured JSON logs + db.query spans)
  └─ migration -> jobs.Service.Enqueue (span: queue.enqueue)
                    │  traceparent diserialisasi ke migration_jobs.trace_json
                    ▼
              PostgreSQL Queue (migration_jobs)
                    │  Worker extract trace_json -> context
                    ▼
              Worker (span: queue.process)
                ├─ databaseLock (span: database.lock)
                └─ migration.Service.Execute (span: migration.execute)
                     └─ migration.Runner (span: migration.sql / migration.batch / migration.revert_batch)
                          ├─ sql.started
                          ├─ sql.finished (duration_ms, rows, status, error, sqlstate)
                          └─ sql.transaction_commit / sql.transaction_rollback

Telemetry Outputs:
  - Always-on: Structured JSON logs ke stdout (tidak disimpan di DB platform)
  - Default tracing (tanpa collector): consoleExporter JSON span ke stdout
  - Bonus (jika OTEL_EXPORTER_OTLP_ENDPOINT di-set):
      OTLP/HTTP -> OpenTelemetry Collector (:4318)
                     ├─ Traces -> Tempo (:4317)
                     └─ Logs   -> Loki (/otlp)
  - Prometheus Metrics:
      GET /metrics (backend :8080/metrics, worker :9091/metrics) -> Prometheus -> Grafana (:3000)
```

---

## 2. Redaction & Keamanan Log

Implementasi di `src/internal/telemetry/telemetry.go` memastikan data sensitif tidak pernah tertulis ke log maupun span:
- Key sensitif (`password`, `passwd`, `secret`, `token`, `cookie`, `authorization`, `csrf`, `credential`, `encryption_key`, `private_key`, `dsn`, `database_url`, `api_key`, `session_id`, `session_token`, `session_cookie`) otomatis diganti menjadi `[REDACTED]`.
- Nilai string yang memuat DSN berkredensial (`postgres://user:pass@host`), `Bearer <token>`, atau pasangan `password=...` / `token=...` otomatis disamarkan.
- Parameter SQL dicatat sebagai placeholder `[REDACTED]` sesuai jumlah parameter melalui `telemetry.RedactParams(...)`.
- Middleware HTTP hanya mencatat `method` dan `path` tanpa query string, cookie, maupun body.

---

## 3. Contoh Output Structured Log JSON & Korelasi Trace

### A. Aktivitas HTTP, Auth, Access Control, dan Admin
```json
{"time":"2026-11-26T12:10:01.102Z","level":"INFO","event":"auth.login_succeeded","service":"platform-api","request_id":"a1b2c3d4e5f60708","actor":"8c1d4c3e-2b0f-4f91-9b11-111111111111","username":"user1","status":"success","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7"}
{"time":"2026-11-26T12:10:05.418Z","level":"WARN","event":"access.denied","service":"platform-api","request_id":"9f8e7d6c5b4a3210","actor":"8c1d4c3e-2b0f-4f91-9b11-111111111111","database_id":"db-prod-01","path":"/api/v1/databases/db-prod-01/migrations","required":"VIEW_EDIT","status":"forbidden","trace_id":"7a1c9e2f4b8d10356c2e8a4f9d0b1e23","span_id":"11a2b3c4d5e6f708"}
{"time":"2026-11-26T12:10:12.884Z","level":"INFO","event":"admin.group_updated","service":"platform-api","request_id":"1234abcd5678ef90","actor":"00000001-0000-0000-0000-000000000001","group_id":"grp-eng-01","group_name":"Engineering","leader_id":"8c1d4c3e-2b0f-4f91-9b11-111111111111","status":"success","trace_id":"9c8b7a6d5e4f3210abcdef1234567890","span_id":"aabbccddeeff0011"}
```

### B. Query Read-Only (Explorer)
```json
{"time":"2026-11-26T12:11:02.001Z","level":"INFO","event":"sql.started","service":"platform-api","request_id":"f0e1d2c3b4a59687","actor":"8c1d4c3e-2b0f-4f91-9b11-111111111111","database_id":"db-prod-01","operation":"explorer.run_query","read_only":true,"query":"SELECT id, email FROM customers LIMIT 50;","parameters":[],"status":"started","trace_id":"d4c3b2a1908f7e6d5c4b3a291807f6e5","span_id":"1234567890abcdef"}
{"time":"2026-11-26T12:11:02.015Z","level":"INFO","event":"sql.finished","service":"platform-api","request_id":"f0e1d2c3b4a59687","actor":"8c1d4c3e-2b0f-4f91-9b11-111111111111","database_id":"db-prod-01","operation":"explorer.run_query","read_only":true,"query":"SELECT id, email FROM customers LIMIT 50;","parameters":[],"duration_ms":14.2,"rows":50,"status":"success","error":"","sqlstate":"","trace_id":"d4c3b2a1908f7e6d5c4b3a291807f6e5","span_id":"1234567890abcdef"}
{"time":"2026-11-26T12:11:02.016Z","level":"INFO","event":"sql.transaction_rollback","service":"platform-api","request_id":"f0e1d2c3b4a59687","actor":"8c1d4c3e-2b0f-4f91-9b11-111111111111","database_id":"db-prod-01","operation":"explorer.run_query","status":"success","trace_id":"d4c3b2a1908f7e6d5c4b3a291807f6e5","span_id":"0987654321fedcba"}
```

### C. Migration Batch — Mulai, Hasil Eksekusi, dan Commit/Rollback Terpisah (Satu `trace_id` dari API hingga Worker)
```json
{"time":"2026-11-26T12:12:00.100Z","level":"INFO","event":"job.enqueued","service":"platform-api","request_id":"req-mig-01","actor":"8c1d4c3e-2b0f-4f91-9b11-111111111111","database_id":"db-prod-01","job_id":"job-101","execution_id":"exec-201","status":"PENDING","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"1000000000000002"}
{"time":"2026-11-26T12:12:00.310Z","level":"INFO","event":"job.lock_acquired","service":"platform-worker","actor":"8c1d4c3e-2b0f-4f91-9b11-111111111111","database_id":"db-prod-01","job_id":"job-101","execution_id":"exec-201","status":"success","duration_ms":1.8,"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"1000000000000004"}
{"time":"2026-11-26T12:12:00.320Z","level":"INFO","event":"migration.execution_started","service":"platform-worker","actor":"8c1d4c3e-2b0f-4f91-9b11-111111111111","database_id":"db-prod-01","migration_id":"mig-301","job_id":"job-101","execution_id":"exec-201","attempt":1,"version":0,"action_type":"FORWARD","mode":"batch","query":"UPDATE orders SET status = 'archived' WHERE id IN (SELECT id FROM orders WHERE id > {{CURSOR}} ORDER BY id LIMIT {{BATCH_SIZE}});","status":"started","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"1000000000000005"}
{"time":"2026-11-26T12:12:00.322Z","level":"INFO","event":"sql.started","service":"platform-worker","actor":"8c1d4c3e-2b0f-4f91-9b11-111111111111","database_id":"db-prod-01","migration_id":"mig-301","job_id":"job-101","execution_id":"exec-201","attempt":1,"batch":1,"operation":"migration.batch","query":"UPDATE orders SET status = 'archived' WHERE id IN (SELECT id FROM orders WHERE id > 0 ORDER BY id LIMIT 1000);","parameters":["[REDACTED]","[REDACTED]"],"status":"started","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"1000000000000006"}
{"time":"2026-11-26T12:12:00.410Z","level":"INFO","event":"sql.finished","service":"platform-worker","actor":"8c1d4c3e-2b0f-4f91-9b11-111111111111","database_id":"db-prod-01","migration_id":"mig-301","job_id":"job-101","execution_id":"exec-201","attempt":1,"batch":1,"operation":"migration.batch","query":"UPDATE orders SET status = 'archived' WHERE id IN (SELECT id FROM orders WHERE id > 0 ORDER BY id LIMIT 1000);","parameters":["[REDACTED]","[REDACTED]"],"duration_ms":88.4,"rows":1000,"status":"success","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"1000000000000006"}
{"time":"2026-11-26T12:12:00.415Z","level":"INFO","event":"sql.transaction_commit","service":"platform-worker","actor":"8c1d4c3e-2b0f-4f91-9b11-111111111111","database_id":"db-prod-01","migration_id":"mig-301","job_id":"job-101","execution_id":"exec-201","attempt":1,"batch":1,"operation":"migration.batch","rows":1000,"status":"success","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"1000000000000006"}
```

### D. Contoh Output OpenTelemetry Span (Console Exporter)
```json
{"time":"2026-11-26T12:12:00.105Z","level":"INFO","event":"trace.span","service":"platform-api","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"1000000000000001","parent_span_id":"0000000000000000","name":"http.request","kind":"server","status":"Unset","started_at":"2026-11-26T12:12:00.095Z","duration_ms":9.8,"attributes":{"http.request.method":"POST","http.response.status_code":200,"request.id":"req-mig-01","url.path":"/api/v1/databases/db-prod-01/migrations/mig-301/execute"}}
{"time":"2026-11-26T12:12:00.105Z","level":"INFO","event":"trace.span","service":"platform-api","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"1000000000000002","parent_span_id":"1000000000000001","name":"queue.enqueue","kind":"producer","status":"Unset","started_at":"2026-11-26T12:12:00.098Z","duration_ms":4.1,"attributes":{"actor.id":"8c1d4c3e-2b0f-4f91-9b11-111111111111","database.id":"db-prod-01","execution.id":"exec-201","job.id":"job-101"}}
{"time":"2026-11-26T12:12:00.420Z","level":"INFO","event":"trace.span","service":"platform-worker","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"1000000000000003","parent_span_id":"1000000000000002","name":"queue.process","kind":"consumer","status":"Unset","started_at":"2026-11-26T12:12:00.305Z","duration_ms":114.2,"attributes":{"actor.id":"8c1d4c3e-2b0f-4f91-9b11-111111111111","database.id":"db-prod-01","execution.id":"exec-201","job.id":"job-101"}}
{"time":"2026-11-26T12:12:00.420Z","level":"INFO","event":"trace.span","service":"platform-worker","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"1000000000000004","parent_span_id":"1000000000000003","name":"database.lock","kind":"internal","status":"Unset","started_at":"2026-11-26T12:12:00.308Z","duration_ms":1.8,"attributes":{"database.id":"db-prod-01"}}
{"time":"2026-11-26T12:12:00.420Z","level":"INFO","event":"trace.span","service":"platform-worker","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"1000000000000006","parent_span_id":"1000000000000005","name":"migration.batch","kind":"internal","status":"Unset","started_at":"2026-11-26T12:12:00.321Z","duration_ms":94.1,"attributes":{"database.id":"db-prod-01","db.rows":1000,"execution.id":"exec-201","migration.batch_number":1}}
```

---

## 4. Prometheus Metrics (`/metrics`)

Endpoint `GET /metrics` hanya menggunakan label ber-kardinalitas rendah (`status`, `reason`, `result`, `mode`, `action`, `pool`, `state`) dan **tidak pernah** menggunakan `user_id`, `job_id`, `execution_id`, `database_id`, atau raw `sql` sebagai label.

Contoh output:
```prometheus
# HELP platform_migration_jobs_by_status Current number of migration jobs in each queue status.
# TYPE platform_migration_jobs_by_status gauge
platform_migration_jobs_by_status{status="BLOCKED"} 0
platform_migration_jobs_by_status{status="FAILED"} 1
platform_migration_jobs_by_status{status="PENDING"} 2
platform_migration_jobs_by_status{status="RETRY_WAIT"} 0
platform_migration_jobs_by_status{status="RUNNING"} 1
platform_migration_jobs_by_status{status="SUCCEEDED"} 5
# HELP platform_migration_jobs_total Total migration jobs by lifecycle event status.
# TYPE platform_migration_jobs_total counter
platform_migration_jobs_total{status="enqueued"} 6
platform_migration_jobs_total{status="succeeded"} 5
# HELP platform_migration_job_retries_total Total migration job retries scheduled by worker.
# TYPE platform_migration_job_retries_total counter
platform_migration_job_retries_total{reason="retry_resume"} 1
# HELP platform_migration_queue_wait_seconds Seconds a migration job waited in queue before worker execution started.
# TYPE platform_migration_queue_wait_seconds histogram
platform_migration_queue_wait_seconds_sum 0.25
platform_migration_queue_wait_seconds_count 1
# HELP platform_migration_duration_seconds Execution duration of migration attempts in seconds.
# TYPE platform_migration_duration_seconds histogram
platform_migration_duration_seconds_sum{action="forward",mode="batch",status="succeeded"} 1.2
platform_migration_duration_seconds_count{action="forward",mode="batch",status="succeeded"} 1
# HELP platform_migration_batches_processed_total Total migration batches processed by action and status.
# TYPE platform_migration_batches_processed_total counter
platform_migration_batches_processed_total{action="forward",status="committed"} 1
# HELP platform_migration_rows_processed_total Total rows processed by migration batches.
# TYPE platform_migration_rows_processed_total counter
platform_migration_rows_processed_total{action="forward"} 1000
# HELP platform_db_pool_connections Current database connection pool counts by pool role and state.
# TYPE platform_db_pool_connections gauge
platform_db_pool_connections{pool="platform",state="idle"} 2
platform_db_pool_connections{pool="platform",state="in_use"} 2
platform_db_pool_connections{pool="platform",state="max_open"} 10
platform_db_pool_connections{pool="platform",state="open"} 4
# HELP platform_db_pool_wait_count_total Total number of connections waited for by pool role.
# TYPE platform_db_pool_wait_count_total counter
platform_db_pool_wait_count_total{pool="platform"} 3
# HELP platform_db_pool_wait_duration_seconds_total Total time blocked waiting for a new database connection in seconds.
# TYPE platform_db_pool_wait_duration_seconds_total counter
platform_db_pool_wait_duration_seconds_total{pool="platform"} 0.15
```

---

## 5. Cara Menjalankan Bonus Observability Stack (OTel Collector + Tempo + Loki + Prometheus + Grafana)

Jalankan dari root repository:

```bash
docker compose -f docker-compose.yml -f docker-compose.observability.yml up -d --build
```

Atau sertakan worker terpisah dengan profile `jobs`:

```bash
docker compose --profile jobs -f docker-compose.yml -f docker-compose.observability.yml up -d --build
```

Akses layanan observability:
- **Grafana:** <http://localhost:3000> (auto-provisioned dashboard: *DB Management Platform — Queue, Migration & DB Pool*, default login `admin` / `admin` atau anonymous Admin).
  - Korelasi **Log <-> Trace**: di **Explore -> Loki**, klik link **TraceID** pada baris log untuk langsung melompat ke waterfall trace di **Tempo**; dari **Tempo**, klik **Logs for this span** untuk melihat log yang berkorelasi.
- **Prometheus:** <http://localhost:9090>
- **Metrics Backend:** <http://localhost:28080/metrics>
- **OTel Collector OTLP HTTP:** `http://localhost:4318`
