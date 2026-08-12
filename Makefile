VERSION := $(shell git describe --tags --always --dirty)
SCRIPTS := scripts

.PHONY: build clean test test-coverage cuse db-query mcp

build: cuse db-query

cuse:
	cd cmd/cuse && go build -ldflags "-X main.version=$(VERSION)" -o ../../$(SCRIPTS)/cuse .

db-query:
	cd cmd/db-query && go build -o ../../$(SCRIPTS)/db-query .
	cd cmd/db-query && go build -o ../../$(SCRIPTS)/db-query-mcp ./cmd/db-query-mcp

mcp:
	cd cmd/db-query && go run ./cmd/db-query-mcp

clean:
	rm -f $(SCRIPTS)/cuse $(SCRIPTS)/db-query $(SCRIPTS)/db-query-mcp

test:
	go -C cmd/cuse test -cover ./...
	go -C cmd/db-query test -cover -race ./...

test-coverage:
	go -C cmd/cuse test -coverprofile=coverage.out ./...
	go -C cmd/db-query test -coverprofile=coverage.out ./...
	go -C cmd/db-query tool cover -html=coverage.out -o coverage.html
