#!/usr/bin/env bash
# NetSpec first-time host setup: data tree, desired-state + alerts samples, .env seeding.
#
# NetSpec compares live state to desired state and raises alerts on mismatch. config/alerts.yaml
# routes those events (Apprise-API + url_env secrets in .env). The compose stack includes
# netspec-apprise so APPRISE_API_URL=http://127.0.0.1:8086 works by default.
#
# UX: no flags needed for a runnable sample. NETSPEC_DATA_DIR is taken from .env when set;
# otherwise /opt/netspec or ~/netspec-data. Split device YAMLs are always seeded unless opted out.
#
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
ENV_FILE="$REPO_ROOT/.env"

# Prompts only with --interactive (default is quiet, scriptable).
INTERACTIVE=0
FORCE=0
DATA_DIR=""
SNMP_COMMUNITY=""
SEED_EXAMPLE_DEVICES=1

usage() {
	cat <<'EOF'
Bootstrap NETSPEC_DATA_DIR, copy desired-state + alerts samples, merge .env — run with no options
for a working sample stack (split devices + ingest 57501 vs Telegraf 57500).

Options:
  --data-dir PATH           Override NETSPEC_DATA_DIR (else use .env or default path)
  --snmp-community STR      SNMP v2c community in .env (default: public)
  --no-seed-example-devices Do not copy config/devices/*.yaml (you must define devices yourself)
  --interactive             Prompt for data dir and SNMP community
  --non-interactive         Same as default (kept for CI scripts)
  --force                   Replace top-level seeded YAML from repo; reset .env from .env.example (backup)
  -h, --help                This help

Examples:
  ./scripts/setup-netspec.sh
  sudo ./scripts/setup-netspec.sh --data-dir /opt/netspec
EOF
}

log() { printf '%s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

can_write_dir() {
	local d=$1
	[[ -n "$d" ]] || return 1
	if [[ -d "$d" ]]; then
		[[ -w "$d" ]]
	else
		local parent
		parent=$(dirname "$d")
		[[ -w "$parent" ]]
	fi
}

pick_default_data_dir() {
	if can_write_dir "/opt/netspec"; then
		printf '%s' "/opt/netspec"
	else
		printf '%s' "${HOME}/netspec-data"
	fi
}

prompt() {
	local def=$2
	local r
	if [[ "$INTERACTIVE" -eq 1 ]]; then
		read -r -p "$1 [$def]: " r
		if [[ -z "${r:-}" ]]; then
			printf '%s' "$def"
		else
			printf '%s' "$r"
		fi
	else
		printf '%s' "$def"
	fi
}

read_netspec_data_dir_from_env_file() {
	local f=$1
	[[ -f "$f" ]] || return 1
	local line val
	line=$(grep -E '^[[:space:]]*NETSPEC_DATA_DIR=' "$f" | tail -n 1) || return 1
	val=${line#NETSPEC_DATA_DIR=}
	val=${val%$'\r'}
	val=${val#\"}
	val=${val%\"}
	val=${val#\'}
	val=${val%\'}
	[[ -n "$val" ]] || return 1
	printf '%s' "$val"
}

count_split_device_yamls() {
	local d=$1
	local n=0
	local f
	shopt -s nullglob
	for f in "$d"/*.yaml; do
		((n++)) || true
	done
	shopt -u nullglob
	printf '%s' "$n"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		-h|--help) usage; exit 0 ;;
		--interactive) INTERACTIVE=1 ;;
		--non-interactive) INTERACTIVE=0 ;;
		--no-seed-example-devices) SEED_EXAMPLE_DEVICES=0 ;;
		--force) FORCE=1 ;;
		--data-dir) DATA_DIR=$2; shift ;;
		--snmp-community) SNMP_COMMUNITY=$2; shift ;;
		*) die "unknown option: $1 (try --help)" ;;
	esac
	shift
done

[[ -f "$REPO_ROOT/docker-compose.yml" ]] || die "docker-compose.yml not found under $REPO_ROOT (run this from a NetSpec checkout)."

if [[ -z "$DATA_DIR" ]]; then
	if env_dd=$(read_netspec_data_dir_from_env_file "$ENV_FILE" 2>/dev/null); then
		DATA_DIR="$env_dd"
		log "Using NETSPEC_DATA_DIR from $ENV_FILE: $DATA_DIR"
	elif [[ "$INTERACTIVE" -eq 1 ]]; then
		DATA_DIR=$(prompt "NETSPEC_DATA_DIR (persistent config + data root)" "$(pick_default_data_dir)")
	else
		DATA_DIR=$(pick_default_data_dir)
		log "NETSPEC_DATA_DIR (default): $DATA_DIR"
	fi
fi

if [[ -z "$SNMP_COMMUNITY" ]]; then
	if [[ "$INTERACTIVE" -eq 1 ]]; then
		SNMP_COMMUNITY=$(prompt "SNMP community (v2c)" "public")
	else
		SNMP_COMMUNITY="public"
	fi
fi

mkdir -p "$DATA_DIR/config" "$DATA_DIR/data" "$DATA_DIR/apprise-config" "$DATA_DIR/mdt-sidecar"
mkdir -p "$DATA_DIR/config/devices" "$DATA_DIR/data/devices"

sample_desired="$REPO_ROOT/config/desired-state.yaml"
sample_alerts="$REPO_ROOT/config/alerts.yaml"
sample_env="$REPO_ROOT/.env.example"
sample_devices_dir="$REPO_ROOT/config/devices"

[[ -f "$sample_desired" ]] || die "missing $sample_desired"
[[ -f "$sample_env" ]] || die "missing $sample_env"
[[ -f "$sample_alerts" ]] || die "missing $sample_alerts"
[[ -d "$sample_devices_dir" ]] || die "missing $sample_devices_dir"

dest_desired="$DATA_DIR/config/desired-state.yaml"
if [[ ! -f "$dest_desired" || "$FORCE" -eq 1 ]]; then
	cp "$sample_desired" "$dest_desired"
	log "Installed $dest_desired (sample)."
else
	log "Leaving existing $dest_desired (use --force to replace)."
fi

# Default new deployments to 57501 for NetSpec ingest (57500 is commonly Telegraf input).
tmp=$(mktemp)
awk '
BEGIN { in_ingest=0 }
/^[[:space:]]*ingest:[[:space:]]*$/ { in_ingest=1; print; next }
in_ingest && /^[[:space:]]*port:[[:space:]]*57500[[:space:]]*$/ {
	sub(/57500/, "57501")
	print
	next
}
in_ingest && /^[^[:space:]]/ { in_ingest=0 }
{ print }
' "$dest_desired" >"$tmp" && mv "$tmp" "$dest_desired"

dest_alerts="$DATA_DIR/config/alerts.yaml"
if [[ ! -f "$dest_alerts" || "$FORCE" -eq 1 ]]; then
	cp "$sample_alerts" "$dest_alerts"
	log "Installed $dest_alerts (sample — set url_env secrets in .env to match channels)."
else
	log "Leaving existing $dest_alerts (use --force to replace)."
fi

repo_device_count=0
for _src_chk in "$sample_devices_dir"/*.yaml; do
	[[ -e "$_src_chk" ]] || continue
	((repo_device_count++)) || true
done
[[ "$repo_device_count" -gt 0 ]] || die "No *.yaml files under $sample_devices_dir — repo may be incomplete."

if [[ "$SEED_EXAMPLE_DEVICES" -eq 1 ]]; then
	# Always ensure split files exist whenever the sample desired-state uses devices: {} —
	# including when someone copied only desired-state.yaml/alerts.yaml by hand.
	prior_n=$(count_split_device_yamls "$DATA_DIR/config/devices")
	for src in "$sample_devices_dir"/*.yaml; do
		[[ -e "$src" ]] || continue
		dst="$DATA_DIR/config/devices/$(basename "$src")"
		if [[ ! -f "$dst" || "$FORCE" -eq 1 ]]; then
			cp "$src" "$dst"
			log "Installed $dst (example split device)."
		fi
	done
	after_n=$(count_split_device_yamls "$DATA_DIR/config/devices")
	if [[ "$prior_n" -eq 0 && "$after_n" -gt 0 ]]; then
		log "Split device YAMLs were missing under config/devices — sample stack needs these because desired-state.yaml uses devices: {}."
	fi
	[[ "$after_n" -gt 0 ]] || die "Failed to seed $DATA_DIR/config/devices — NetSpec would exit with \"no devices configured\"."
else
	log "Skipping example split-device seed (--no-seed-example-devices)."
	log "WARNING: sample desired-state uses devices: {} — without split files NetSpec will not start unless you define devices elsewhere."
fi

env_dest="$REPO_ROOT/.env"
if [[ ! -f "$env_dest" ]]; then
	cp "$sample_env" "$env_dest"
	log "Created $env_dest from .env.example"
elif [[ "$FORCE" -eq 1 ]]; then
	cp "$env_dest" "${env_dest}.bak.$(date +%Y%m%d%H%M%S)" || true
	cp "$sample_env" "$env_dest"
	log "Reset $env_dest from .env.example (--force; backup kept)"
else
	log "Keeping existing $env_dest — updating NETSPEC_DATA_DIR and SNMP_COMMUNITY in place."
fi

if grep -q '^NETSPEC_DATA_DIR=' "$env_dest" 2>/dev/null; then
	tmp=$(mktemp)
	sed "s|^NETSPEC_DATA_DIR=.*|NETSPEC_DATA_DIR=$DATA_DIR|" "$env_dest" >"$tmp" && mv "$tmp" "$env_dest"
else
	printf '\n# Added by setup-netspec.sh — persistent tree for compose volumes\nNETSPEC_DATA_DIR=%s\n' "$DATA_DIR" >>"$env_dest"
fi

esc_snmp=$(printf '%s' "$SNMP_COMMUNITY" | sed 's/[\/&|]/\\&/g')
tmp=$(mktemp)
sed "s/^SNMP_COMMUNITY=.*/SNMP_COMMUNITY=$esc_snmp/" "$env_dest" >"$tmp" && mv "$tmp" "$env_dest"

# Keep translator target aligned with new-deploy ingest default unless overridden later.
if grep -q '^NETSPEC_INGEST_PORT=' "$env_dest" 2>/dev/null; then
	tmp=$(mktemp)
	sed "s/^NETSPEC_INGEST_PORT=.*/NETSPEC_INGEST_PORT=57501/" "$env_dest" >"$tmp" && mv "$tmp" "$env_dest"
else
	printf 'NETSPEC_INGEST_PORT=57501\n' >>"$env_dest"
fi

# Telegraf container writes /sidecar as uid/gid 999.
if chown -R 999:999 "$DATA_DIR/mdt-sidecar" && chmod 775 "$DATA_DIR/mdt-sidecar"; then
	:
elif command -v sudo >/dev/null 2>&1 && sudo -n chown -R 999:999 "$DATA_DIR/mdt-sidecar" && sudo -n chmod 775 "$DATA_DIR/mdt-sidecar"; then
	log "Adjusted $DATA_DIR/mdt-sidecar ownership via passwordless sudo (uid 999)."
else
	printf '%s\n' "WARNING: could not chown/chmod $DATA_DIR/mdt-sidecar for uid 999 — Telegraf will restart-loop with permission denied." >&2
	printf '%s\n' "         Fix: sudo chown -R 999:999 \"$DATA_DIR/mdt-sidecar\" && sudo chmod 775 \"$DATA_DIR/mdt-sidecar\"" >&2
fi

log ""
log "Next steps (repo root: $REPO_ROOT):"
log "  1. Edit $DATA_DIR/config/desired-state.yaml (devices, telemetry_mode, ingest)."
log "  2. Edit $DATA_DIR/config/alerts.yaml and $env_dest (APPRISE_* / url_env — destinations for drift alerts)."
log "  3. docker compose pull && docker compose up -d   # sample UI: http://127.0.0.1:8088 — Apprise on :8086"
log "  4. Reload after YAML edits: POST /api/reload or the dashboard button."
log "  5. Optional check: ./scripts/validate-netspec-stack.sh --project-dir $REPO_ROOT"
log ""
log "If you deploy with Komodo, Portainer, or similar: keep this checkout as the Compose"
log "project root (needs ./tools/sidecar next to docker-compose.yml) and point NETSPEC_DATA_DIR"
log "at the same path you used here. Details: README «Komodo, Portainer, and similar UIs»."
log ""
log "NETSPEC_DATA_DIR=$DATA_DIR"

validator="$REPO_ROOT/scripts/validate-netspec-stack.sh"
if [[ -x "$validator" ]]; then
	log ""
	log "Running post-setup validation..."
	"$validator" --project-dir "$REPO_ROOT"
else
	log ""
	log "Validation script not found/executable at $validator; skipping post-setup validation."
fi
