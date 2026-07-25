VERSION := $(shell git describe --tags --always --dirty)
OUTPUT := ../scripts/cuse

.PHONY: build clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(OUTPUT) .

clean:
	rm -f $(OUTPUT)

test:
	go test -cover ./...

test-coverage:
	go test -coverprofile=./coverage.out ./...
	go tool cover -html=coverage.out -o ./coverage.html
	open ./coverage.html