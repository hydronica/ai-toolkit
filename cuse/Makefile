VERSION := $(shell git describe --tags --always --dirty)
OUTPUT := ../scripts/cuse

.PHONY: build clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(OUTPUT) .

clean:
	rm -f $(OUTPUT)
