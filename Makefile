VERSION := $(shell git describe --tags --always --dirty)
GOBIN := $(CURDIR)/scripts

.PHONY: build clean test test-coverage cuse db-query

build: cuse db-query

cuse:
	go -C cmd/cuse build -ldflags "-X main.version=$(VERSION)" -o $(GOBIN)/cuse .

db-query:
	go -C cmd/db-query build -ldflags "-X main.version=$(VERSION)" -o $(GOBIN)/db-query .

clean:
	rm -f $(GOBIN)/cuse $(GOBIN)/db-query

test:
	go -C cmd/cuse test -cover ./...
	go -C cmd/db-query test -cover -race ./...

test-coverage:
	go -C cmd/cuse test -coverprofile=coverage.out ./...
	go -C cmd/db-query test -coverprofile=coverage.out ./...
	go -C cmd/db-query tool cover -html=coverage.out -o coverage.html
