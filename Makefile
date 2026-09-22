# NetSpec — local Docker workflows (parity with containerized prod)
#
# Requires: Docker Compose v2, repo root as build context.
# Set NETSPEC_DATA_DIR to your runtime tree (config/, data/, apprise-config/, mdt-sidecar/).
#
# Examples:
#   export NETSPEC_DATA_DIR=/opt/netspec
#   make docker-rebuild && make docker-up   # after Go or translator changes
#   make docker-build-mdt                   # native Go MDT sidecar (dual-run on :57502)
#   make docker-up-mdt                      # Telegraf off; Go MDT on host :57500
#   make docker-up                          # start only (no rebuild)
#
# Requires Docker Compose v2 (`docker compose`). Use `sudo -E` if your user
# needs elevated rights for the daemon; keep NETSPEC_* exports with `-E`.

COMPOSE_LOCAL  := docker compose -f docker-compose.yml -f docker-compose.build-local.yml
COMPOSE_MDT_GO := $(COMPOSE_LOCAL) -f docker-compose.mdt-go.yml
COMPOSE_MDT_CUTOVER := $(COMPOSE_MDT_GO) -f docker-compose.mdt-go-cutover.yml

export NETSPEC_LOCAL_COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
export NETSPEC_LOCAL_BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
export NETSPEC_LOCAL_VERSION    ?= dev

.PHONY: setup proto docker-build-netspec docker-build-mdt-translator docker-build-mdt docker-rebuild docker-up docker-up-mdt docker-down docker-logs-netspec

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

docker-build-mdt-translator:
	$(COMPOSE_LOCAL) build netspec-mdt-translator

docker-build-mdt:
	$(COMPOSE_MDT_GO) build netspec-mdt

docker-rebuild: docker-build-netspec docker-build-mdt-translator

docker-up:
	$(COMPOSE_LOCAL) up -d

docker-up-mdt:
	$(COMPOSE_MDT_CUTOVER) up -d --build

docker-down:
	$(COMPOSE_LOCAL) down

docker-logs-netspec:
	$(COMPOSE_LOCAL) logs -f netspec-netspec
