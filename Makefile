.PHONY: build test clean run docker-build docker-up docker-down

build:
	go build -o netspec ./cmd/netspec

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
