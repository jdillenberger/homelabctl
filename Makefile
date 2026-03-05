BINARY := homelabctl
PKG := github.com/jdillenberger/homelabctl
CMD := ./cmd/homelabctl

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
  -X main.version=$(VERSION) \
  -X main.commit=$(COMMIT) \
  -X main.date=$(DATE)

GOFLAGS := -trimpath

.PHONY: all build build-pi build-pi32 run test lint fmt vet clean install help dev deploy-pi release

all: build ## Build for current platform

build: ## Build for current platform
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o bin/$(BINARY) $(CMD)

build-pi: ## Cross-compile for Raspberry Pi (arm64)
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o bin/$(BINARY)-linux-arm64 $(CMD)

build-pi32: ## Cross-compile for Raspberry Pi (armv7)
	GOOS=linux GOARCH=arm GOARM=7 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o bin/$(BINARY)-linux-armv7 $(CMD)

run: build ## Run with ARGS (e.g., make run ARGS="apps list")
	./bin/$(BINARY) $(ARGS)

test: ## Run tests
	go test ./... -v -count=1

lint: ## Run golangci-lint
	golangci-lint run ./...

fmt: ## Format code
	gofmt -s -w .
	goimports -w .

vet: ## Run go vet
	go vet ./...

clean: ## Remove build artifacts
	rm -rf bin/

install: build ## Install to /usr/local/bin
	sudo cp bin/$(BINARY) /usr/local/bin/$(BINARY)

dev: ## Run with live reload (requires air)
	air -- $(ARGS)

deploy-pi: build-pi ## Deploy to Pi (PI_HOST=pi@hostname)
	scp bin/$(BINARY)-linux-arm64 $(PI_HOST):/usr/local/bin/$(BINARY)
	ssh $(PI_HOST) "sudo systemctl restart $(BINARY) 2>/dev/null || true"

release: ## Build for all platforms with goreleaser
	goreleaser build --snapshot --clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
