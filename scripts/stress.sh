#!/usr/bin/env bash
set -Eeuo pipefail
ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
case "${1:-run}" in
  -h|--help)
    cat <<'HELP'
Usage: ./scripts/stress.sh [smoke|run|down]
Requires Docker Compose v2 and OpenSSL. No local k6, Python, Node or Go needed.
Uses isolated project if3110-stress; does not touch development queues/data.
run: same and different database tests (default, ~10 minutes plus preparation/drain).
smoke: short end-to-end verification; not evidence of stress capacity.
down: stop stress containers, preserving volumes/results.
Environment: MODE=both|same|different API_RATE=10 JOB_RATE=12 HOLD_SECONDS=60
RAMP_SECONDS=15 DRAIN_SECONDS=300 JOB_ROWS=1000 JOB_DELAY=2 MAX_VUS=20
API_P95_MS=500 ERROR_RATE=0.01 JOBS_CONCURRENCY=4
Results: tests/artifacts/k6/<timestamp>-<mode>/
HELP
    exit 0 ;;
  run|smoke|down) action="${1:-run}" ;;
  *) echo 'Unknown argument; use --help' >&2; exit 2 ;;
esac
cd "$ROOT_DIR"
command -v openssl >/dev/null
command -v docker >/dev/null
docker compose version >/dev/null
mkdir -p tests/artifacts/k6
key_file="$ROOT_DIR/tests/artifacts/k6/.key"
if [[ ! -f "$key_file" ]]; then (umask 077; openssl rand -base64 32 > "$key_file"); fi
export STRESS_DB_KEY="$(cat "$key_file")"
export STRESS_UID="$(id -u)" STRESS_GID="$(id -g)"
export STRESS_OUTPUT="$ROOT_DIR/tests/artifacts/k6"
compose=(docker compose -p if3110-stress -f "$ROOT_DIR/tests/k6/compose.yml")
if [[ "$action" == down ]]; then "${compose[@]}" down; exit 0; fi
if [[ "$action" == smoke ]]; then
  export HOLD_SECONDS=3 RAMP_SECONDS=1 JOB_RATE=60 API_RATE=2 JOB_DELAY=0.1
fi
case "${MODE:-both}" in
  both) modes=(same different) ;;
  same|different) modes=("$MODE") ;;
  *) echo 'MODE must be both, same, or different' >&2; exit 2 ;;
esac
"${compose[@]}" up -d --build --wait postgres target-a target-b backend worker
status=0
for mode in "${modes[@]}"; do
  export MODE="$mode"
  export STRESS_OUTPUT="$ROOT_DIR/tests/artifacts/k6/$(date -u +%Y%m%dT%H%M%S)-$mode"
  mkdir -p "$STRESS_OUTPUT"
  "${compose[@]}" run --rm --no-deps --user "$STRESS_UID:$STRESS_GID" tools python prepare.py
  # Always reconcile server jobs, including when thresholds cause k6 exit 99
  "${compose[@]}" run --rm --no-deps k6 run --out json=/results/raw.json /scripts/stress.js || status=1
  "${compose[@]}" run --rm --no-deps --user "$STRESS_UID:$STRESS_GID" tools python report.py || status=1
  "${compose[@]}" logs --no-color backend worker > "$STRESS_OUTPUT/application.log"
  docker info --format 'CPUs={{.NCPU}} MemoryBytes={{.MemTotal}} OS={{.OperatingSystem}}' > "$STRESS_OUTPUT/environment.txt"
  printf 'Report: %s/report.md\n' "$STRESS_OUTPUT"
done
exit "$status"
