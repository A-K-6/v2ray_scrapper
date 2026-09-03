APP := v2rays
LEGACY_APP := v2ray-scrapper
GO_CACHE := $(CURDIR)/.cache/go-build
GO_MOD_CACHE := $(CURDIR)/.cache/go-mod
GO_ENV := GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE)
VERSION ?= dev

.PHONY: help init setup run build test test-init e2e-public lint fmt clean up down logs ps restart docker-build docker-up docker-down docker-logs install tui doctor serve refresh release-snapshot

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

init: ## Interactively create .env, config.yaml, and runtime directories
	@sh scripts/init.sh $(ARGS)

setup: ## Download Go modules
	$(GO_ENV) go mod download

run: ## Run the API locally
	$(GO_ENV) go run . serve

build: ## Build a small production binary
	mkdir -p bin
	$(GO_ENV) go build -trimpath -ldflags="-s -w -X main.buildVersion=$(VERSION)" -o bin/$(APP) .
	ln -sf $(APP) bin/$(LEGACY_APP)

test: ## Run unit tests with the race detector
	$(GO_ENV) go test -race ./...
	@$(MAKE) --no-print-directory test-init

test-init: ## Test the project initializer
	@sh scripts/init_test.sh

e2e-public: docker-build ## Test real public feeds, APIs, and restart within the cold-start budget
	sh scripts/e2e-public.sh

lint: ## Run Go static analysis
	$(GO_ENV) go vet ./...

fmt: ## Format Go sources
	gofmt -w *.go

clean: ## Remove local build artifacts
	rm -rf bin .cache

docker-build: ## Build the container
	docker compose build

up: init ## Initialize, rebuild, and start the service
	DOCKER_UID=$$(id -u) DOCKER_GID=$$(id -g) docker compose up -d --build

down: ## Stop the service
	docker compose down

logs: ## Follow service logs
	docker compose logs -f

ps: ## Show service status
	docker compose ps

restart: ## Restart the service
	docker compose restart

docker-up: up ## Alias for up

docker-down: down ## Alias for down

docker-logs: logs ## Alias for logs

install: build ## Install v2rays to ~/.local/bin
	mkdir -p $(HOME)/.local/bin
	cp bin/$(APP) $(HOME)/.local/bin/$(APP)
	chmod +x $(HOME)/.local/bin/$(APP)
	@echo "installed to $(HOME)/.local/bin/$(APP) (ensure it is on PATH)"

tui: build ## Launch the interactive menu
	./bin/$(APP) tui

doctor: build ## Check standalone health
	./bin/$(APP) doctor

serve: build ## Serve the API (standalone, no docker)
	./bin/$(APP) serve

refresh: build ## Run one refresh cycle and export base64
	./bin/$(APP) refresh

release-snapshot: ## Build release archives locally (requires goreleaser)
	goreleaser release --snapshot --clean
