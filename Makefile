BINARY    := wharfeye
MODULE    := github.com/mathesh-me/wharfeye
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X $(MODULE)/cmd/wharfeye/cmd.version=$(VERSION)
GOFLAGS   := -trimpath

.PHONY: all build test lint vet fmt clean install run-tui run-web

all: lint test build

build:
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/wharfeye

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

vet:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -f $(BINARY)
	go clean -cache -testcache

install: build
	cp $(BINARY) $(GOPATH)/bin/$(BINARY) 2>/dev/null || cp $(BINARY) /usr/local/bin/$(BINARY)

run-tui: build
	./$(BINARY)

run-web: build
	./$(BINARY) web
