# NetSpec — local Docker workflows (parity with containerized prod)
#
# Requires: Docker Compose v2, repo root as build context.
# Set NETSPEC_DATA_DIR to your runtime tree (config/, data/, apprise-config/).
#
# Examples:
#   export NETSPEC_DATA_DIR=/opt/netspec
#   make docker-rebuild && make docker-up   # after Go changes
#   make docker-up                          # start only (no rebuild)
#
# Requires Docker Compose v2 (`docker compose`). Use `sudo -E` if your user
# needs elevated rights for the daemon; keep NETSPEC_* exports with `-E`.

COMPOSE_LOCAL := docker compose -f docker-compose.yml -f docker-compose.build-local.yml

export NETSPEC_LOCAL_COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
export NETSPEC_LOCAL_BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
export NETSPEC_LOCAL_VERSION    ?= dev

.PHONY: setup proto docker-build-netspec docker-build-mdt docker-build-mdt-translator docker-rebuild docker-up docker-down docker-logs-netspec

# First-time host bootstrap: data dirs, sample config, .env (see scripts/setup-netspec.sh --help)
setup:
	./scripts/setup-netspec.sh

proto:
	PATH="$(PATH):$(shell go env GOPATH)/bin" protoc --proto_path=proto \
		--go_out=. --go_opt=module=github.com/netspec/netspec \
		--go-grpc_out=. --go-grpc_opt=module=github.com/netspec/netspec \
		proto/telemetry_bis.proto proto/mdt_dialout.proto

docker-build-netspec:
	$(COMPOSE_LOCAL) build netspec-netspec

docker-build-mdt:
	$(COMPOSE_LOCAL) build netspec-mdt

docker-build-mdt-translator:
	$(COMPOSE_LOCAL) -f docker-compose.legacy-mdt.yml build netspec-mdt-translator

docker-rebuild: docker-build-netspec docker-build-mdt

docker-up:
	$(COMPOSE_LOCAL) up -d --remove-orphans

docker-down:
	$(COMPOSE_LOCAL) down --remove-orphans

docker-logs-netspec:
	$(COMPOSE_LOCAL) logs -f netspec-netspec
