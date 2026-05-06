#!/usr/bin/env bash
# Validate NetSpec host/runtime settings before compose deploy/restart.
#
# Fails fast when common drift causes startup issues:
# - NETSPEC_DATA_DIR missing in .env (falls back to defaults unexpectedly)
# - global.ingest.port mismatch with NETSPEC_INGEST_PORT
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

if [[ -d "$DATA_DIR/config/devices" ]]; then
	if ! ls -1 "$DATA_DIR/config/devices"/*.yaml >/dev/null 2>&1; then
		die "no split-device YAML files in $DATA_DIR/config/devices"
	fi
	ok "split-device files present in config/devices"
else
	warn "split-device directory missing: $DATA_DIR/config/devices (monolithic devices may still be valid)"
fi

# Telegraf writes /sidecar/decoded.json as uid 999. If composer/translator creates these as root first,
# Telegraf crash-loops and nothing listens on 57500 (MDT dial-out silently fails).
SIDECAR_DIR="$DATA_DIR/mdt-sidecar"
if [[ -d "$SIDECAR_DIR" ]]; then
	for f in "$SIDECAR_DIR/decoded.json" "$SIDECAR_DIR/forwarder.log"; do
		[[ -e "$f" ]] || continue
		uid=$(stat -c '%u' "$f" 2>/dev/null || true)
		if [[ -n "$uid" && "$uid" != "999" ]]; then
			warn "mdt-sidecar file owned by uid $uid (Telegraf needs 999): $f — fix: sudo chown -R 999:999 \"$SIDECAR_DIR\" && sudo docker restart netspec-telegraf-mdt netspec-mdt-translator"
		fi
	done
	ok "mdt-sidecar checked (decoded.json / forwarder.log must be uid 999 when present)"
fi

ok "validation passed"
