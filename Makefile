# NetSpec — local Docker workflows (parity with containerized prod)
#
# Requires: Docker Compose v2, repo root as build context.
# Set NETSPEC_DATA_DIR to your runtime tree (config/, data/, apprise-config/, mdt-sidecar/).
#
# Examples:
#   export NETSPEC_DATA_DIR=/opt/netspec
#   make docker-rebuild && make docker-up          # after Go changes
#   make docker-up                                  # start only (no rebuild)
#
# Telemetry sidecar (telegraf-mdt + mdt-translator):
#   make docker-rebuild && make docker-up-telemetry
#
# Requires Docker Compose v2 (`docker compose`). Use `sudo -E` if your user
# needs elevated rights for the daemon; keep NETSPEC_* exports with `-E`.

COMPOSE_BASE   := docker compose -f docker-compose.yml -f docker-compose.build-local.yml
COMPOSE_TELEM  := docker compose -f docker-compose.yml -f docker-compose.dev.yml -f docker-compose.build-local.yml

export NETSPEC_LOCAL_COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
export NETSPEC_LOCAL_BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
export NETSPEC_LOCAL_VERSION    ?= dev

.PHONY: docker-build-netspec docker-build-mdt-translator docker-rebuild docker-up docker-down docker-up-telemetry docker-down-telemetry docker-logs-netspec

docker-build-netspec:
	$(COMPOSE_BASE) build netspec

docker-build-mdt-translator:
	$(COMPOSE_TELEM) build mdt-translator

docker-rebuild: docker-build-netspec docker-build-mdt-translator

docker-up:
	$(COMPOSE_BASE) up -d

docker-down:
	$(COMPOSE_BASE) down

docker-up-telemetry:
	$(COMPOSE_TELEM) up -d

docker-down-telemetry:
	$(COMPOSE_TELEM) down

docker-logs-netspec:
	$(COMPOSE_BASE) logs -f netspec
