.PHONY: build test vet check api

build:
	mkdir -p bin
	go build -trimpath -o bin/argus ./cmd/argus
	go build -trimpath -o bin/argus-api ./cmd/argus-api

test:
	go test ./...

vet:
	go vet ./...

check: test vet

api:
	go run ./cmd/argus-api
