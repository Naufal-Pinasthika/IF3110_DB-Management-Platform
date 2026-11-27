#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
RESET=false
ENV_TEMP=''

usage() {
  cat <<'EOF'
Usage: ./scripts/setup.sh [--reset]

Build and start the complete development stack, including the migration worker,
using Docker Compose.
Requires Docker with Compose v2 and OpenSSL; local Go, Node.js and psql
are not required. Run from any directory.

  --reset   Delete this Compose project's containers and database volumes
            before rebuilding. ALL project database data will be lost.
  -h, --help
            Show this help without changing anything.

Normal runs preserve .env, its encryption key, and existing database data.
EOF
}

for argument in "$@"; do
  case "$argument" in
    --reset) RESET=true ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'Unknown argument: %s\n' "$argument" >&2; usage >&2; exit 2 ;;
  esac
done

cleanup() {
  if [[ -n "$ENV_TEMP" && -f "$ENV_TEMP" ]]; then
    rm -f -- "$ENV_TEMP"
  fi
}
trap cleanup EXIT
trap 'printf "Setup failed at line %s. Check the error above; database data is preserved unless --reset was selected.\n" "$LINENO" >&2' ERR

for executable in docker openssl; do
  if ! command -v "$executable" >/dev/null 2>&1; then
    printf 'Required command missing: %s. Install it and run setup again.\n' "$executable" >&2
    exit 1
  fi
done
docker compose version >/dev/null
if ! docker info >/dev/null 2>&1; then
  printf 'Docker is unavailable. Start Docker and ensure your user can access it.\n' >&2
  exit 1
fi

cd -- "$ROOT_DIR"
umask 077
if [[ ! -f .env ]]; then
  cp .env.example .env
  printf 'Created .env from .env.example.\n'
fi

# Read simple dotenv values without executing .env as a shell script.
read_env_value() {
  local name="$1" line value='' matched
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    if [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?${name}[[:space:]]*=(.*)$ ]]; then
      matched="${BASH_REMATCH[2]}"
      matched="${matched#"${matched%%[![:space:]]*}"}"
      matched="${matched%%[[:space:]]#*}"
      matched="${matched%"${matched##*[![:space:]]}"}"
      if [[ "$matched" == \"*\" || "$matched" == \'*\' ]]; then
        matched="${matched:1:${#matched}-2}"
      fi
      value="$matched"
    fi
  done < .env
  printf '%s' "$value"
}

key="${DB_CREDENTIAL_ENCRYPTION_KEY:-$(read_env_value DB_CREDENTIAL_ENCRYPTION_KEY)}"
if [[ -z "$key" || "$key" == 'your-base64-encoded-32-byte-key-here' ]]; then
  export SETUP_ENCRYPTION_KEY
  SETUP_ENCRYPTION_KEY="$(openssl rand -base64 32)"
  ENV_TEMP="$(mktemp "$ROOT_DIR/.env.setup.XXXXXX")"
  awk '
    /^[[:space:]]*(export[[:space:]]+)?DB_CREDENTIAL_ENCRYPTION_KEY[[:space:]]*=/ {
      if (!found++) print "DB_CREDENTIAL_ENCRYPTION_KEY=" ENVIRON["SETUP_ENCRYPTION_KEY"]
      next
    }
    { print }
    END {
      if (!found) print "DB_CREDENTIAL_ENCRYPTION_KEY=" ENVIRON["SETUP_ENCRYPTION_KEY"]
    }
  ' .env > "$ENV_TEMP"
  mv -- "$ENV_TEMP" .env
  ENV_TEMP=''
  key="$SETUP_ENCRYPTION_KEY"
  unset SETUP_ENCRYPTION_KEY
  printf 'Generated a database credential encryption key in .env.\n'
fi
if [[ ! "$key" =~ ^[A-Za-z0-9+/]{43}=$ ]]; then
  printf 'DB_CREDENTIAL_ENCRYPTION_KEY must be a base64-encoded 32-byte key. Existing value was preserved.\n' >&2
  exit 1
fi
# Match Compose's environment precedence, including a newly generated key.
export DB_CREDENTIAL_ENCRYPTION_KEY="$key"
unset key

compose=(docker compose --profile jobs --project-directory "$ROOT_DIR" --env-file "$ROOT_DIR/.env" -f "$ROOT_DIR/docker-compose.yml")
"${compose[@]}" config --quiet

if "$RESET"; then
  printf 'Removing this project\047s containers, volumes, and locally built images...\n'
  "${compose[@]}" down --volumes --remove-orphans --rmi local
fi

printf 'Building React, the Go server, and the worker in Docker...\n'
"${compose[@]}" build backend worker

printf 'Starting the platform database...\n'
"${compose[@]}" up -d postgres

# Wait once for PostgreSQL to accept TCP connections before applying migrations.
# The official image only enables TCP after its initialization scripts finish.
db_deadline=$((SECONDS + 180))
until "${compose[@]}" exec -T -e PGPASSWORD=root postgres \
  psql -X -h 127.0.0.1 -U admin -d platform_db -c 'SELECT 1' >/dev/null 2>&1; do
  if (( SECONDS >= db_deadline )); then
    printf 'PostgreSQL did not become available. Check docker compose logs postgres.\n' >&2
    exit 1
  fi
  sleep 2
done

# Compose initializes the base schema and development accounts on a new volume.
# Reapply the additive, idempotent upgrades for both new and existing volumes.
for migration in "$ROOT_DIR"/db/platform/[0-9]*.sql; do
  name="${migration##*/}"
  case "$name" in
    000001_*|*_dev_*|*_reset_dev_*) continue ;;
  esac
  printf 'Applying %s...\n' "$name"
  "${compose[@]}" exec -T postgres \
    psql -X -v ON_ERROR_STOP=1 -U admin -d platform_db < "$migration"
done

printf 'Starting target databases, the web server, and the migration worker...\n'
"${compose[@]}" up -d \
  target-db-1 target-db-2 target-db-3 target-mysql backend worker

origin="${PUBLIC_ORIGIN:-$(read_env_value PUBLIC_ORIGIN)}"
printf '\nWeb starting at: %s\n' "${origin:-http://localhost:6767}"
printf 'Development login: admin / AdminDemo-2026! (on a fresh database)\n'
printf 'Logs: docker compose logs -f backend worker\nStop: docker compose --profile jobs down\n'
