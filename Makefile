.PHONY: build test clean run docker-build docker-up docker-down

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -X github.com/netspec/netspec/internal/version.Version=$(VERSION) \
          -X github.com/netspec/netspec/internal/version.Commit=$(COMMIT) \
          -X github.com/netspec/netspec/internal/version.BuildDate=$(BUILD_DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o netspec ./cmd/netspec

test:
	go test ./...

clean:
	rm -f netspec

run: build
	./netspec -config ./config/desired-state.yaml

docker-build:
	docker-compose build

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f netspec
