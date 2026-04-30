VERSION ?= dev
LDFLAGS := -ldflags "-X github.com/WTomas/sufleur-cli/internal/cli.Version=$(VERSION)"

.PHONY: build test lint clean

build:
	go build $(LDFLAGS) -o dist/sufleur ./cmd/sufleur

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf dist/
