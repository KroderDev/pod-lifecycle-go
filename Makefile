.PHONY: test lint vet

all: test lint vet

test:
	go test -cover ./...

vet:
	go vet ./...

lint:
	golangci-lint run
