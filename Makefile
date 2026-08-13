# NetSpec — local Docker workflows (parity with containerized prod)
#
# Requires: Docker Compose v2, repo root as build context.
# Set NETSPEC_DATA_DIR to your runtime tree (config/, data/, apprise-config/, mdt-sidecar/).
#
# Examples:
#   export NETSPEC_DATA_DIR=/opt/netspec
#   make docker-rebuild && make docker-up   # after Go or translator changes
#   make docker-up                          # start only (no rebuild)
#   make graph-dev-up                       # dedicated Graph lab VM (default ports + Graph)
#   make graph-lab-up                       # parallel lab beside prod on same Docker host
#
# Requires Docker Compose v2 (`docker compose`). Use `sudo -E` if your user
# needs elevated rights for the daemon; keep NETSPEC_* exports with `-E`.

COMPOSE_LOCAL  := docker compose -f docker-compose.yml -f docker-compose.build-local.yml
COMPOSE_GRAPH  := docker compose -f docker-compose.yml -f docker-compose.build-local.yml --profile graph
COMPOSE_LAB    := docker compose --env-file .env.graph-lab -f docker-compose.yml -f docker-compose.build-local.yml --profile graph

export NETSPEC_LOCAL_COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
export NETSPEC_LOCAL_BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
export NETSPEC_LOCAL_VERSION    ?= dev

.PHONY: setup docker-build-netspec docker-build-mdt-translator docker-rebuild docker-up docker-down docker-logs-netspec \
	graph-dev-up graph-dev-down \
	setup-graph-lab graph-lab-build graph-lab-up graph-lab-down graph-lab-logs

# First-time host bootstrap: data dirs, sample config, .env (see scripts/setup-netspec.sh --help)
setup:
	./scripts/setup-netspec.sh

docker-build-netspec:
	$(COMPOSE_LOCAL) build netspec-netspec

docker-build-mdt-translator:
	$(COMPOSE_LOCAL) build netspec-mdt-translator

docker-rebuild: docker-build-netspec docker-build-mdt-translator

docker-up:
	$(COMPOSE_LOCAL) up -d

docker-down:
	$(COMPOSE_LOCAL) down

docker-logs-netspec:
	$(COMPOSE_LOCAL) logs -f netspec-netspec

# --- Dedicated Graph lab VM (default ports; see docs/NETSPECGRAPH.md) ---

graph-dev-up: docker-rebuild
	$(COMPOSE_GRAPH) up -d

graph-dev-down:
	$(COMPOSE_GRAPH) down

# --- Parallel lab on same host as production (alternate ports) ---

setup-graph-lab:
	./scripts/setup-graph-lab.sh

graph-lab-build:
	@test -f .env.graph-lab || (echo "missing .env.graph-lab — run: make setup-graph-lab" >&2; exit 1)
	$(COMPOSE_LAB) build netspec-netspec netspec-mdt-translator

graph-lab-up: graph-lab-build
	@test -f .env.graph-lab || (echo "missing .env.graph-lab — run: make setup-graph-lab" >&2; exit 1)
	$(COMPOSE_LAB) up -d

graph-lab-down:
	@test -f .env.graph-lab || (echo "missing .env.graph-lab — run: make setup-graph-lab" >&2; exit 1)
	$(COMPOSE_LAB) down

graph-lab-logs:
	@test -f .env.graph-lab || (echo "missing .env.graph-lab — run: make setup-graph-lab" >&2; exit 1)
	$(COMPOSE_LAB) logs -f netspec-netspec netspec-graph netspec-victoriametrics netspec-telegraf-mdt
