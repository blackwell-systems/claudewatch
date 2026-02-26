.PHONY: build test lint vet clean snapshot install

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/claudewatch ./cmd/claudewatch

test:
	go test ./... -v

lint:
	golangci-lint run ./...

vet:
	go vet ./...

clean:
	rm -rf bin/ dist/

snapshot:
	goreleaser release --snapshot --clean

install: build
	cp bin/claudewatch $(GOPATH)/bin/claudewatch 2>/dev/null || cp bin/claudewatch /usr/local/bin/claudewatch
