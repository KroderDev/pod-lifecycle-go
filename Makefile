.PHONY: test lint vet

all:  vet test lint

test:
	go test -cover ./...

vet:
	go vet ./...

lint:
	golangci-lint run
