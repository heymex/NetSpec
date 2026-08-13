#!/usr/bin/env bash
# Bootstrap an isolated NetSpec + NetSpecGraph lab stack beside production.
#
# Creates NETSPEC_DATA_DIR, seeds config (sample or cloned), writes .env.graph-lab.
# Does not start containers — run `make graph-lab-up` after this.
#
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
ENV_OUT="$REPO_ROOT/.env.graph-lab"
EXAMPLE="$REPO_ROOT/.env.graph-lab.example"

DATA_DIR=""
CLONE_FROM=""
FORCE=0

usage() {
	cat <<'EOF'
Bootstrap an isolated Graph lab NetSpec instance (separate data dir + ports).

Options:
  --data-dir PATH           Lab NETSPEC_DATA_DIR (default: /opt/netspec-graphlab or ~/netspec-graphlab-data)
  --clone-config-from PATH  Copy config/ from an existing NetSpec tree (e.g. /opt/netspec/config)
  --force                   Overwrite .env.graph-lab and re-seed top-level sample YAML
  -h, --help                This help

Examples:
  ./scripts/setup-graph-lab.sh
  ./scripts/setup-graph-lab.sh --data-dir /opt/netspec-graphlab --clone-config-from /opt/netspec/config
  make graph-lab-up          # after setup
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
	if can_write_dir "/opt/netspec-graphlab"; then
		printf '%s' "/opt/netspec-graphlab"
	else
		printf '%s' "${HOME}/netspec-graphlab-data"
	fi
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--data-dir)
		DATA_DIR=${2:-}
		[[ -n "$DATA_DIR" ]] || die "--data-dir needs a path"
		shift 2
		;;
	--clone-config-from)
		CLONE_FROM=${2:-}
		[[ -n "$CLONE_FROM" ]] || die "--clone-config-from needs a path"
		shift 2
		;;
	--force)
		FORCE=1
		shift
		;;
	-h|--help)
		usage
		exit 0
		;;
	*)
		die "unknown option: $1"
		;;
	esac
done

[[ -f "$EXAMPLE" ]] || die "missing $EXAMPLE"
[[ -f "$REPO_ROOT/docker-compose.yml" ]] || die "run from a NetSpec checkout"

if [[ -z "$DATA_DIR" ]]; then
	DATA_DIR=$(pick_default_data_dir)
fi

log "Lab data dir: $DATA_DIR"
mkdir -p "$DATA_DIR/config" "$DATA_DIR/config/devices" "$DATA_DIR/data" "$DATA_DIR/data/devices" \
	"$DATA_DIR/apprise-config" "$DATA_DIR/mdt-sidecar" "$DATA_DIR/victoria-metrics"

if [[ -n "$CLONE_FROM" ]]; then
	[[ -d "$CLONE_FROM" ]] || die "clone source not a directory: $CLONE_FROM"
	log "Cloning config from $CLONE_FROM → $DATA_DIR/config"
	# Preserve lab independence: copy, do not symlink (edits must not hit prod).
	if command -v rsync >/dev/null 2>&1; then
		rsync -a --delete "$CLONE_FROM"/ "$DATA_DIR/config"/
	else
		rm -rf "$DATA_DIR/config"
		mkdir -p "$DATA_DIR/config"
		cp -R "$CLONE_FROM"/. "$DATA_DIR/config"/
	fi
else
	# Seed samples when empty / forced (same idea as setup-netspec.sh).
	if [[ ! -f "$DATA_DIR/config/desired-state.yaml" || "$FORCE" -eq 1 ]]; then
		cp "$REPO_ROOT/config/desired-state.yaml" "$DATA_DIR/config/desired-state.yaml"
		log "Seeded desired-state.yaml"
	fi
	if [[ ! -f "$DATA_DIR/config/alerts.yaml" || "$FORCE" -eq 1 ]]; then
		cp "$REPO_ROOT/config/alerts.yaml" "$DATA_DIR/config/alerts.yaml"
		log "Seeded alerts.yaml (consider silencing channels for lab)"
	fi
	if [[ ! -f "$DATA_DIR/config/rules.yaml" && -f "$REPO_ROOT/config/rules.yaml" ]]; then
		cp "$REPO_ROOT/config/rules.yaml" "$DATA_DIR/config/rules.yaml"
	fi
	if [[ -d "$REPO_ROOT/config/devices" ]]; then
		shopt -s nullglob
		for f in "$REPO_ROOT/config/devices"/*.yaml; do
			base=$(basename "$f")
			if [[ ! -f "$DATA_DIR/config/devices/$base" || "$FORCE" -eq 1 ]]; then
				cp "$f" "$DATA_DIR/config/devices/$base"
			fi
		done
		shopt -u nullglob
	fi
fi

if [[ -f "$ENV_OUT" && "$FORCE" -eq 0 ]]; then
	log "Keeping existing $ENV_OUT (use --force to regenerate from example)"
	# Always refresh NETSPEC_DATA_DIR to match this run.
	tmp=$(mktemp)
	if grep -q '^NETSPEC_DATA_DIR=' "$ENV_OUT"; then
		sed "s|^NETSPEC_DATA_DIR=.*|NETSPEC_DATA_DIR=$DATA_DIR|" "$ENV_OUT" >"$tmp" && mv "$tmp" "$ENV_OUT"
	else
		printf '\nNETSPEC_DATA_DIR=%s\n' "$DATA_DIR" >>"$ENV_OUT"
		rm -f "$tmp"
	fi
else
	if [[ -f "$ENV_OUT" ]]; then
		cp "$ENV_OUT" "$ENV_OUT.bak.$(date +%Y%m%d%H%M%S)"
		log "Backed up existing .env.graph-lab"
	fi
	sed "s|^NETSPEC_DATA_DIR=.*|NETSPEC_DATA_DIR=$DATA_DIR|" "$EXAMPLE" >"$ENV_OUT"
	log "Wrote $ENV_OUT"
fi

# Telegraf uid 999 needs write on mdt-sidecar
if chown -R 999:999 "$DATA_DIR/mdt-sidecar" 2>/dev/null && chmod 775 "$DATA_DIR/mdt-sidecar" 2>/dev/null; then
	log "mdt-sidecar ownership set to uid 999"
elif command -v sudo >/dev/null 2>&1 && sudo -n chown -R 999:999 "$DATA_DIR/mdt-sidecar" && sudo -n chmod 775 "$DATA_DIR/mdt-sidecar"; then
	log "mdt-sidecar ownership set to uid 999 (sudo)"
else
	printf '%s\n' "WARNING: could not chown $DATA_DIR/mdt-sidecar to uid 999" >&2
	printf '%s\n' "         Fix: sudo chown -R 999:999 \"$DATA_DIR/mdt-sidecar\"" >&2
fi

log ""
log "Next:"
log "  1. Review $ENV_OUT (ports, SNMP_COMMUNITY, auth)."
log "  2. Silence lab alerts if you cloned prod alerts.yaml."
log "  3. make graph-lab-up"
log "  4. Open http://127.0.0.1:8188 (NetSpec) and http://127.0.0.1:8190 (Graph)."
log "  5. vmui: http://127.0.0.1:18428/vmui"
log ""
log "Telemetry: add a *second* MDT receiver on subscription 251 pointing at"
log "  <lab-host-ip>:57510  (do not steal production's :57500 receiver)."
log "See docs/NETSPECGRAPH.md § Lab instance."
