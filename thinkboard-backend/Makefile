.PHONY: run build test vet

run:
	go run ./cmd/gateway

build:
	go build -o bin/gateway ./cmd/gateway

test:
	go test ./...

vet:
	go vet ./...
