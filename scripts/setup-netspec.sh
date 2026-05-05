#!/usr/bin/env bash
# NetSpec first-time host setup: data tree, desired-state + alerts samples, .env seeding.
#
# NetSpec compares live state to desired state and raises alerts on mismatch. config/alerts.yaml
# routes those events (Apprise-API + url_env secrets in .env). The compose stack includes
# netspec-apprise so APPRISE_API_URL=http://127.0.0.1:8086 works by default.
#
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)

INTERACTIVE=1
FORCE=0
DATA_DIR=""
SNMP_COMMUNITY=""
NON_INTERACTIVE=0

usage() {
	cat <<'EOF'
Bootstrap NETSPEC_DATA_DIR, copy desired-state.yaml + alerts.yaml samples, merge .env.

NetSpec is built to flag drift from desired state; alerts.yaml + Apprise destination URLs are
part of a normal deployment (edit channels and .env secrets after install).

Options:
  --data-dir PATH           NETSPEC_DATA_DIR (default: /opt/netspec or $HOME/netspec-data if unwritable)
  --snmp-community STR      SNMP v2c community written into .env (default: public)
  --non-interactive         Non-prompting (uses defaults above)
  --force                   Overwrite seeded configs from repo; resets .env from .env.example (backup first)
  -h, --help                This help

Examples:
  ./scripts/setup-netspec.sh
  sudo ./scripts/setup-netspec.sh --data-dir /opt/netspec --non-interactive
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

while [[ $# -gt 0 ]]; do
	case "$1" in
		-h|--help) usage; exit 0 ;;
		--non-interactive) NON_INTERACTIVE=1; INTERACTIVE=0 ;;
		--force) FORCE=1 ;;
		--data-dir) DATA_DIR=$2; shift ;;
		--snmp-community) SNMP_COMMUNITY=$2; shift ;;
		*) die "unknown option: $1 (try --help)" ;;
	esac
	shift
done

[[ -f "$REPO_ROOT/docker-compose.yml" ]] || die "docker-compose.yml not found under $REPO_ROOT (run this from a NetSpec checkout)."

if [[ -z "$DATA_DIR" ]]; then
	if [[ "$INTERACTIVE" -eq 1 ]]; then
		DATA_DIR=$(prompt "NETSPEC_DATA_DIR (persistent config + data root)" "$(pick_default_data_dir)")
	else
		DATA_DIR=$(pick_default_data_dir)
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

sample_desired="$REPO_ROOT/config/desired-state.yaml"
sample_alerts="$REPO_ROOT/config/alerts.yaml"
sample_env="$REPO_ROOT/.env.example"

[[ -f "$sample_desired" ]] || die "missing $sample_desired"
[[ -f "$sample_env" ]] || die "missing $sample_env"
[[ -f "$sample_alerts" ]] || die "missing $sample_alerts"

dest_desired="$DATA_DIR/config/desired-state.yaml"
if [[ ! -f "$dest_desired" || "$FORCE" -eq 1 ]]; then
	cp "$sample_desired" "$dest_desired"
	log "Installed $dest_desired (sample)."
else
	log "Leaving existing $dest_desired (use --force to replace)."
fi

dest_alerts="$DATA_DIR/config/alerts.yaml"
if [[ ! -f "$dest_alerts" || "$FORCE" -eq 1 ]]; then
	cp "$sample_alerts" "$dest_alerts"
	log "Installed $dest_alerts (sample — set url_env secrets in .env to match channels)."
else
	log "Leaving existing $dest_alerts (use --force to replace)."
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

log ""
log "Next steps (repo root: $REPO_ROOT):"
log "  1. Edit $DATA_DIR/config/desired-state.yaml (devices, telemetry_mode, ingest)."
log "  2. Edit $DATA_DIR/config/alerts.yaml and $env_dest (APPRISE_* / url_env — destinations for drift alerts)."
log "  3. docker compose pull && docker compose up -d   # includes Apprise-API on :8086"
log "  4. Reload after YAML edits: POST /api/reload or the dashboard button."
log ""
log "If you deploy with Komodo, Portainer, or similar: keep this checkout as the Compose"
log "project root (needs ./tools/sidecar next to docker-compose.yml) and point NETSPEC_DATA_DIR"
log "at the same path you used here. Details: README «Komodo, Portainer, and similar UIs»."
log ""
log "NETSPEC_DATA_DIR=$DATA_DIR"
