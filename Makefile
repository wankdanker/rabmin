.PHONY: all ui build test test-short check dev devcluster devcluster-down openapi docker clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/wankdanker/rabmin/internal/version.Version=$(VERSION) -X github.com/wankdanker/rabmin/internal/version.Commit=$(shell git rev-parse --short HEAD 2>/dev/null)

all: ui build

ui:
	cd ui && pnpm install --frozen-lockfile && pnpm run build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/rabmin ./cmd/rabmin
	CGO_ENABLED=0 go build -trimpath -o bin/rabmin-devcluster ./cmd/rabmin-devcluster

test:
	go test ./...

test-short:
	go test -short ./...

check:
	go vet ./...
	test -z "$$(gofmt -l .)"
	cd ui && pnpm run check

# Regenerate the OpenAPI document and the UI's typed client after API changes.
openapi:
	go run ./cmd/rabmin openapi > docs/openapi.json
	cd ui && pnpm run gen:api

# Local RabbitMQ clusters in Docker with demo traffic (3-node east, 1-node west).
devcluster:
	go run ./cmd/rabmin-devcluster --demo

devcluster-down:
	go run ./cmd/rabmin-devcluster --down

# API server on :8080; run `cd ui && pnpm dev` alongside for the UI with hot reload on :5173.
dev:
	go run ./cmd/rabmin --data ./data --poll 2s

docker:
	docker build -t rabmin:$(VERSION) .

clean:
	rm -rf bin web/dist/_app web/dist/index.html web/dist/favicon.svg ui/.svelte-kit
