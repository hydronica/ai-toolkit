.PHONY: build test clean mcp

build:
	go build -o bin/db-query .
	go build -o bin/db-query-mcp ./cmd/db-query-mcp

mcp:
	go run ./cmd/db-query-mcp

test:
	go test -cover -race ./...

clean:
	rm -rf bin/
