SHELL := /bin/sh

BINARY := urbino
GO := go

.PHONY: fmt vet test build generate generate-check sqlc-generate sqlc-check

fmt:
	gofmt -w $$(find cmd internal tests api -type f -name '*.go' -print)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

build:
	$(GO) build -o $(BINARY) ./cmd/urbino

generate:
	$(GO) generate ./api ./internal/storage/postgres

generate-check:
	@set -e; tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; cp api/admin_gen.go "$$tmp/admin_gen.go"; cp -R internal/storage/postgres/generated "$$tmp/generated"; $(GO) generate ./api ./internal/storage/postgres; cmp -s api/admin_gen.go "$$tmp/admin_gen.go"; diff -ru "$$tmp/generated" internal/storage/postgres/generated; diff -ru migrations internal/storage/migrate/migrations

sqlc-generate:
	$(GO) generate ./internal/storage/postgres

sqlc-check:
	$(MAKE) sqlc-generate
