VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build web test check clean

# The binary, with the UI built and embedded.
build: web
	go build -trimpath -ldflags "$(LDFLAGS)" -o agentmd ./cmd/agentmd

web:
	cd web && npm ci && npm run build

test:
	go vet ./...
	go test ./...
	cd web && npm test

check:
	cd web && npm run check

clean:
	rm -f agentmd
	rm -rf dist
