# --- Variables ---
PYTHON := python3
PIP := $(PYTHON) -m pip
VENV := .venv
BIN := $(VENV)/bin
GO := go
XRAY_URL := https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-64.zip
XRAY_BIN := /usr/local/bin/xray
GO_TESTER_DIR := src/go-tester
GO_TESTER_BIN := $(GO_TESTER_DIR)/xray-tester

# Docker command
DOCKER_COMPOSE := docker compose

# --- Colors ---
BLUE := \033[36m
CYAN := \033[36m
GREEN := \033[32m
YELLOW := \033[33m
BOLD := \033[1m
RESET := \033[0m

.PHONY: help venv install build-go setup run test clean lint fmt \
        docker-build docker-up docker-down docker-logs docker-ps docker-restart docker-pull docker-clean

help: ## Show this help message
	@echo -e "\n$(BOLD)V2Ray Scrapper & Tester - Management CLI$(RESET)"
	@echo -e "Usage: make $(BLUE)<target>$(RESET)\n"
	@awk 'BEGIN {FS = ":.*##"; printf "$(BOLD)%-20s$(RESET) %s\n", "Target", "Description"} \
		/^[a-zA-Z0-9_-]+:.*?##/ { printf "  $(CYAN)%-18s$(RESET) %s\n", $$1, $$2 } \
		/^##@/ { printf "\n$(BOLD)%s$(RESET)\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
	@echo ""

##@ Local Development

venv: ## Create virtual environment and install dependencies
	@echo -e "$(YELLOW)Creating virtual environment...$(RESET)"
	$(PYTHON) -m venv $(VENV)
	$(BIN)/pip install --upgrade pip
	$(BIN)/pip install -r requirements.txt
	@echo -e "$(GREEN)Venv created at $(VENV)$(RESET)"

install: ## Install dependencies globally (not recommended)
	$(PIP) install -r requirements.txt

build-go: ## Build the Go-based Xray tester
	@echo -e "$(YELLOW)Building Go tester...$(RESET)"
	cd $(GO_TESTER_DIR) && $(GO) build -o xray-tester main.go
	@echo -e "$(GREEN)Go tester built at $(GO_TESTER_BIN)$(RESET)"

download-xray: ## Download and install Xray core (Linux 64-bit)
	@echo -e "$(YELLOW)Downloading Xray-core...$(RESET)"
	wget -O /tmp/Xray-linux-64.zip $(XRAY_URL)
	sudo unzip -o /tmp/Xray-linux-64.zip -d /usr/local/bin/
	sudo chmod +x /usr/local/bin/xray
	rm /tmp/Xray-linux-64.zip
	@echo -e "$(GREEN)Xray-core installed at $(XRAY_BIN)$(RESET)"

setup: venv build-go ## Full setup: venv + build-go

run: ## Run the application locally with hot reload
	@echo -e "$(GREEN)Starting FastAPI application...$(RESET)"
	export PYTHONPATH=$$(pwd)/src && $(BIN)/uvicorn main:app --app-dir src --host 0.0.0.0 --port 8084 --reload

test: ## Run unit tests
	@echo -e "$(YELLOW)Running tests...$(RESET)"
	export PYTHONPATH=$$(pwd)/src && $(BIN)/python -m unittest discover tests

lint: ## Run linting (requires ruff)
	@if ! $(BIN)/pip show ruff > /dev/null; then $(BIN)/pip install ruff; fi
	$(BIN)/ruff check src

fmt: ## Run formatting (requires ruff)
	@if ! $(BIN)/pip show ruff > /dev/null; then $(BIN)/pip install ruff; fi
	$(BIN)/ruff format src

clean: ## Cleanup temporary files and artifacts
	@echo -e "$(YELLOW)Cleaning up...$(RESET)"
	rm -rf $(VENV)
	rm -f $(GO_TESTER_BIN)
	find . -type d -name "__pycache__" -exec rm -rf {} +
	rm -f src/Country.mmdb
	@echo -e "$(GREEN)Cleanup complete.$(RESET)"

##@ Docker Operations

docker-up: ## Start services in background
	@echo -e "$(GREEN)Starting services in Docker...$(RESET)"
	$(DOCKER_COMPOSE) up -d

docker-down: ## Stop and remove containers
	@echo -e "$(YELLOW)Stopping services...$(RESET)"
	$(DOCKER_COMPOSE) down

docker-build: ## Build or rebuild services
	@echo -e "$(YELLOW)Building Docker images...$(RESET)"
	$(DOCKER_COMPOSE) build

docker-logs: ## View output from containers
	$(DOCKER_COMPOSE) logs -f

docker-ps: ## List containers
	$(DOCKER_COMPOSE) ps

docker-restart: ## Restart services
	@echo -e "$(YELLOW)Restarting services...$(RESET)"
	$(DOCKER_COMPOSE) restart

docker-pull: ## Pull service images
	$(DOCKER_COMPOSE) pull

docker-clean: ## Remove unused Docker data
	@echo -e "$(YELLOW)Cleaning up Docker resources...$(RESET)"
	$(DOCKER_COMPOSE) down -v --remove-orphans
	docker image prune -f
