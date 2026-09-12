.PHONY: fmt vet test build clean

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

build:
	go build -o urbino ./cmd/urbino

clean:
	go clean
