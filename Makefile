VERSION ?= $(shell \
	base=$$(git describe --tags --always); \
	if git diff --quiet HEAD -- cmd/cuse cmd/db-query Makefile 2>/dev/null \
	   && git diff --cached --quiet -- cmd/cuse cmd/db-query Makefile 2>/dev/null; then \
	  printf '%s' "$$base"; \
	else \
	  hash=$$( { git diff HEAD -- cmd/cuse cmd/db-query Makefile; \
	             git diff --cached -- cmd/cuse cmd/db-query Makefile; } 2>/dev/null \
	           | (command -v shasum >/dev/null 2>&1 && shasum -a 256 || sha256sum) \
	           | awk '{print $$1}' | cut -c1-12); \
	  printf '%s-dirty@%s' "$$base" "$$hash"; \
	fi)
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
