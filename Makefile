SHELL := /bin/sh

BINARY := urbino
GO := go

.PHONY: fmt vet test build generate generate-check

fmt:
	gofmt -w $$(find cmd internal tests api -type f -name '*.go' -print)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

build:
	$(GO) build -o $(BINARY) ./cmd/urbino

generate:
	$(GO) generate ./api

generate-check:
	@tmp=$$(mktemp); trap 'rm -f "$$tmp"' EXIT; cp api/admin_gen.go "$$tmp"; $(GO) generate ./api; cmp -s api/admin_gen.go "$$tmp"
