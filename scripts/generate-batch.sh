#!/usr/bin/env bash
# Deterministic PostgreSQL fixture and non-batched baseline for M2.
set -Eeuo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
reset=false
case "${1:-}" in
  '') ;;
  --reset) reset=true ;;
  -h|--help)
    cat <<'HELP'
Usage: ./scripts/generate-batch.sh [--reset]
Seeds public.batch, then times a whole-table UPDATE and rolls it back.
Requires a running Docker Compose PostgreSQL target (no local psql needed).
Environment: ROWS=500000 TARGET_SERVICE=target-db-1
TARGET_SERVICE accepts target-db-1, target-db-2, or target-db-3.
Existing batch tables are refused unless --reset is supplied.
--reset replaces ONLY public.batch; use after previous test jobs finish.
If the baseline UPDATE is <=1000 ms, rerun with more ROWS and --reset.
The batch migration must use the same dataset as the measured baseline.
HELP
    exit 0 ;;
  *) echo 'Unknown argument; use --help' >&2; exit 2 ;;
esac
if (( $# > 1 )); then
  echo 'Too many arguments; use --help' >&2
  exit 2
fi

rows="${ROWS:-500000}"
service="${TARGET_SERVICE:-target-db-1}"
if [[ ! "$rows" =~ ^[1-9][0-9]*$ ]]; then
  echo 'ROWS must be a positive integer without leading zeros.' >&2
  exit 2
fi
case "$service" in
  target-db-1|target-db-2|target-db-3) ;;
  *) echo 'TARGET_SERVICE must be target-db-1, target-db-2, or target-db-3.' >&2; exit 2 ;;
esac
command -v docker >/dev/null
compose=(docker compose --project-directory "$ROOT_DIR" -f "$ROOT_DIR/docker-compose.yml")
"${compose[@]}" version >/dev/null
printf 'Target: %s / target_db; table: batch; rows: %s; reset: %s\n' "$service" "$rows" "$reset"

# ON_ERROR_STOP and the seed transaction preserve the old fixture on failure.
# Pin search_path so the unqualified SQL used in the UI targets public.batch.
"${compose[@]}" exec -T "$service" psql -X -v ON_ERROR_STOP=1 \
  -v rows="$rows" -v reset="$reset" -U target_db_user -d target_db <<'SQL'
SET search_path TO public;
BEGIN;
\if :reset
DROP TABLE IF EXISTS batch;
\endif
CREATE TABLE batch (
    id bigint PRIMARY KEY,
    processed boolean NOT NULL DEFAULT false,
    payload text NOT NULL
);
INSERT INTO batch (id, processed, payload)
SELECT n, false, repeat(md5(n::text), 8)
FROM generate_series(1::bigint, :rows::bigint) AS g(n);
COMMIT;
ANALYZE batch;

\echo 'Baseline: record the UPDATE time below; M2 requires more than 1000 ms.'
BEGIN;
\timing on
UPDATE batch
SET processed = true,
    payload = md5(payload || id::text);
\timing off
ROLLBACK;

SELECT current_database() AS database, version() AS postgres_version;
SELECT count(*) AS total_rows,
       count(*) FILTER (WHERE NOT processed) AS unprocessed_rows,
       min(id) AS min_id, max(id) AS max_id,
       min(length(payload)) AS min_payload_length,
       max(length(payload)) AS max_payload_length,
       count(*) FILTER (WHERE payload <> repeat(md5(id::text), 8)) AS invalid_payloads
FROM batch;
SQL

printf '\nSeed ready. Expected: %s unprocessed rows, IDs 1..%s, payload length 256, invalid_payloads 0.\n' "$rows" "$rows"
printf 'Use UI batch cursor id and batch size 10000. Record baseline timing before executing.\n'

# SQL UP

# WITH changed AS (
#     UPDATE batch AS t
#     SET processed = true,
#         payload = md5(t.payload || t.id::text)
#     WHERE t.id IN (
#         SELECT id
#         FROM batch
#         WHERE id > {{CURSOR}}
#         ORDER BY id
#         LIMIT {{BATCH_SIZE}}
#     )
#     RETURNING t.id
# )
# SELECT id FROM changed ORDER BY id;

# SQL DOWN

# UPDATE batch
# SET processed = false,
#     payload = repeat(md5(id::text), 8)
# WHERE id > (({{CURSOR}}::bigint - 1) / 10000) * 10000
#   AND id <= {{CURSOR}};

# Verify

# SELECT count(*) AS total,
#        count(*) FILTER (WHERE processed) AS processed,
#        count(*) FILTER (
#            WHERE payload = repeat(md5(id::text), 8)
#        ) AS original
# FROM batch;