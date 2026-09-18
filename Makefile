SHELL := /bin/sh

BINARY := urbino
GO := go

.PHONY: fmt vet test build

fmt:
	gofmt -w $$(find cmd internal tests -type f -name '*.go' -print)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

build:
	$(GO) build -o $(BINARY) ./cmd/urbino
