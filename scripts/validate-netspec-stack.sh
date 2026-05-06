#!/usr/bin/env bash
# Validate NetSpec host/runtime settings before compose deploy/restart.
#
# Fails fast when common drift causes startup issues:
# - NETSPEC_DATA_DIR missing in .env (falls back to defaults unexpectedly)
# - global.ingest.port mismatch with NETSPEC_INGEST_PORT
# - ingest port accidentally set to 57500 (reserved for telegraf listener)
# - runtime split-device directory empty
#
# Usage:
#   ./scripts/validate-netspec-stack.sh
#   ./scripts/validate-netspec-stack.sh --project-dir /etc/komodo/stacks/NetSpec
#
set -euo pipefail

PROJECT_DIR="$(pwd)"

usage() {
	cat <<'EOF'
Validate NetSpec stack wiring before deploy/restart.

Options:
  --project-dir PATH   Path containing docker-compose.yml and .env (default: cwd)
  -h, --help           Show this help
EOF
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		--project-dir)
			PROJECT_DIR="$2"
			shift 2
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			printf 'ERROR: unknown option: %s\n' "$1" >&2
			exit 1
			;;
	esac
done

die() {
	printf 'ERROR: %s\n' "$*" >&2
	exit 1
}

warn() {
	printf 'WARN: %s\n' "$*" >&2
}

ok() {
	printf 'OK: %s\n' "$*"
}

extract_env_value() {
	local file="$1"
	local key="$2"
	local line
	line=$(awk -F'=' -v k="$key" '$1==k {print substr($0, index($0, "=")+1)}' "$file" | tail -n 1)
	printf '%s' "$line"
}

extract_ingest_port() {
	local desired="$1"
	awk '
		/^[[:space:]]*ingest:[[:space:]]*$/ { in_ingest=1; next }
		in_ingest && /^[[:space:]]*[a-zA-Z0-9_]+:[[:space:]]*/ && $1 !~ /^port:/ { next }
		in_ingest && /^[[:space:]]*port:[[:space:]]*[0-9]+[[:space:]]*$/ {
			sub(/^[[:space:]]*port:[[:space:]]*/, "", $0)
			sub(/[[:space:]]*$/, "", $0)
			print $0
			exit
		}
		in_ingest && /^[^[:space:]]/ { in_ingest=0 }
	' "$desired"
}

PROJECT_DIR=$(cd "$PROJECT_DIR" && pwd)
ENV_FILE="$PROJECT_DIR/.env"

[[ -f "$PROJECT_DIR/docker-compose.yml" ]] || die "docker-compose.yml not found in $PROJECT_DIR"
[[ -f "$ENV_FILE" ]] || die ".env not found in $PROJECT_DIR"

DATA_DIR="$(extract_env_value "$ENV_FILE" "NETSPEC_DATA_DIR")"
[[ -n "$DATA_DIR" ]] || die "NETSPEC_DATA_DIR is missing in .env (set it explicitly, e.g. /opt/netspec)"
[[ -d "$DATA_DIR" ]] || die "NETSPEC_DATA_DIR path does not exist: $DATA_DIR"
ok "NETSPEC_DATA_DIR set: $DATA_DIR"

DESIRED_FILE="$DATA_DIR/config/desired-state.yaml"
[[ -f "$DESIRED_FILE" ]] || die "missing runtime desired-state.yaml at $DESIRED_FILE"

INGEST_PORT_YAML="$(extract_ingest_port "$DESIRED_FILE")"
[[ -n "$INGEST_PORT_YAML" ]] || die "could not read global.ingest.port from $DESIRED_FILE"

INGEST_PORT_ENV="$(extract_env_value "$ENV_FILE" "NETSPEC_INGEST_PORT")"
[[ -n "$INGEST_PORT_ENV" ]] || die "NETSPEC_INGEST_PORT missing in .env"

if [[ "$INGEST_PORT_YAML" != "$INGEST_PORT_ENV" ]]; then
	die "ingest port mismatch: desired-state.yaml=$INGEST_PORT_YAML, .env NETSPEC_INGEST_PORT=$INGEST_PORT_ENV"
fi
ok "ingest ports match: $INGEST_PORT_YAML"

if [[ "$INGEST_PORT_YAML" == "57500" ]]; then
	die "global.ingest.port is 57500; reserve 57500 for telegraf and use 57501 for NetSpec ingest"
fi
ok "NetSpec ingest port is not 57500"

if [[ -d "$DATA_DIR/config/devices" ]]; then
	if ! ls -1 "$DATA_DIR/config/devices"/*.yaml >/dev/null 2>&1; then
		die "no split-device YAML files in $DATA_DIR/config/devices"
	fi
	ok "split-device files present in config/devices"
else
	warn "split-device directory missing: $DATA_DIR/config/devices (monolithic devices may still be valid)"
fi

ok "validation passed"
