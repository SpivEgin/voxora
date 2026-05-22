# VoIP Server with LLM Voice Routing — Makefile
# GNU General Public License v3.0 (GPL-3.0)

# Variables
PROJECT_NAME := voip-server
VERSION := 1.0.0
GO_VERSION := 1.21
DART_VERSION := 3.0

# Directories
GO_DIR := voip-server
DART_DIR := agent-web
WEB_DIR := $(DART_DIR)/web
BUILD_DIR := build
DIST_DIR := dist

# Binaries
SERVER_BIN := $(BUILD_DIR)/$(PROJECT_NAME)
TUI_BIN := $(BUILD_DIR)/tui-agent
WEB_OUTPUT := $(WEB_DIR)/main.dart.js

# Docker
DOCKER_COMPOSE := docker-compose.yml
DOCKER_IMAGE := voip-server:latest

# Colors for output
BLUE := \033[34m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
RESET := \033[0m

.PHONY: all help build build-server build-tui build-web test clean install \
        docker-up docker-down docker-logs docker-clean \
        run run-server run-tui run-web \
        lint format deps deps-go deps-dart \
        package release

# Default target
all: build

# Help target
help:
	@echo "$(BLUE)VoIP Server with LLM Voice Routing$(RESET)"
	@echo "=========================================="
	@echo ""
	@echo "$(GREEN)Building:$(RESET)"
	@echo "  make build         — Build all components (server, tui, web)"
	@echo "  make build-server  — Build Go server binary"
	@echo "  make build-tui     — Build CLI/TUI agent binary"
	@echo "  make build-web     — Compile Dart web app to JavaScript"
	@echo ""
	@echo "$(GREEN)Running:$(RESET)"
	@echo "  make run           — Run the server"
	@echo "  make run-tui       — Run the TUI agent"
	@echo "  make run-web       — Serve the web dashboard"
	@echo ""
	@echo "$(GREEN)Testing:$(RESET)"
	@echo "  make test          — Run all Go tests"
	@echo "  make test-verbose  — Run tests with verbose output"
	@echo "  make test-coverage — Run tests with coverage report"
	@echo ""
	@echo "$(GREEN)Docker:$(RESET)"
	@echo "  make docker-up     — Start all services with docker-compose"
	@echo "  make docker-down   — Stop all docker services"
	@echo "  make docker-logs   — View docker logs"
	@echo "  make docker-build  — Build docker images"
	@echo "  make docker-clean  — Remove docker containers and volumes"
	@echo ""
	@echo "$(GREEN)Maintenance:$(RESET)"
	@echo "  make install       — Install dependencies (Go and Dart)"
	@echo "  make deps          — Download Go and Dart dependencies"
	@echo "  make deps-go       — Download Go modules"
	@echo "  make deps-dart     — Get Dart packages"
	@echo "  make lint          — Run linters"
	@echo "  make format        — Format code"
	@echo "  make clean         — Clean build artifacts"
	@echo "  make package       — Create release package"
	@echo ""
	@echo "$(GREEN)Release:$(RESET)"
	@echo "  make release       — Build release binaries for all platforms"
	@echo ""

# =============================================================================
# BUILD TARGETS
# =============================================================================

build: build-server build-tui build-web
	@echo "$(GREEN)✓ All components built successfully$(RESET)"

build-server: deps-go
	@echo "$(BLUE)🔨 Building Go server...$(RESET)"
	@mkdir -p $(BUILD_DIR)
	cd $(GO_DIR) && go build -mod=mod -o ../$(SERVER_BIN) -ldflags="-s -w -X main.Version=$(VERSION)" ./cmd/server
	@echo "$(GREEN)✓ Server binary: $(SERVER_BIN)$(RESET)"

build-tui: deps-go
	@echo "$(BLUE)🔨 Building TUI agent...$(RESET)"
	@mkdir -p $(BUILD_DIR)
	cd $(GO_DIR) && go build -mod=mod -o ../$(TUI_BIN) -ldflags="-s -w" ./cmd/tui-agent
	@echo "$(GREEN)✓ TUI agent binary: $(TUI_BIN)$(RESET)"

build-web: deps-dart
	@echo "$(BLUE)🔨 Compiling Dart web app...$(RESET)"
	cd $(DART_DIR) && dart compile js -O2 -o $(WEB_OUTPUT) web/main.dart
	@echo "$(GREEN)✓ Web app compiled: $(WEB_OUTPUT)$(RESET)"

# =============================================================================
# RUN TARGETS
# =============================================================================

run: build-server
	@echo "$(BLUE)🚀 Starting VoIP Server...$(RESET)"
	./$(SERVER_BIN)

run-tui: build-tui
	@echo "$(BLUE)🚀 Starting TUI Agent...$(RESET)"
	WS_URL="ws://localhost:8080/ws" AGENT_ID="agent_$$(date +%s)" ./$(TUI_BIN)

run-web: build-web
	@echo "$(BLUE)🚀 Serving web dashboard on http://localhost:8081$(RESET)"
	cd $(WEB_DIR) && python3 -m http.server 8081 || python -m http.server 8081

# =============================================================================
# TEST TARGETS
# =============================================================================

test:
	@echo "$(BLUE)🧪 Running tests...$(RESET)"
	cd $(GO_DIR) && go test -mod=mod -v ./internal/...

test-verbose:
	@echo "$(BLUE)🧪 Running tests (verbose)...$(RESET)"
	cd $(GO_DIR) && go test -mod=mod -v -count=1 ./internal/...

test-coverage:
	@echo "$(BLUE)🧪 Running tests with coverage...$(RESET)"
	@mkdir -p $(BUILD_DIR)
	cd $(GO_DIR) && go test -mod=mod -coverprofile=../$(BUILD_DIR)/coverage.out ./internal/...
	cd $(GO_DIR) && go tool cover -html=../$(BUILD_DIR)/coverage.out -o ../$(BUILD_DIR)/coverage.html
	@echo "$(GREEN)✓ Coverage report: $(BUILD_DIR)/coverage.html$(RESET)"

# =============================================================================
# DOCKER TARGETS
# =============================================================================

docker-up:
	@echo "$(BLUE)🐳 Starting Docker services...$(RESET)"
	docker-compose -f $(DOCKER_COMPOSE) up -d
	@echo "$(GREEN)✓ Services started$(RESET)"
	@echo "$(YELLOW)  - Redis: localhost:6379$(RESET)"
	@echo "$(YELLOW)  - Ollama: localhost:11434$(RESET)"
	@echo "$(YELLOW)  - Whisper: localhost:9090$(RESET)"
	@echo "$(YELLOW)  - Piper: localhost:5000$(RESET)"
	@echo "$(YELLOW)  - VoIP Server: localhost:8080$(RESET)"

docker-down:
	@echo "$(BLUE)🐳 Stopping Docker services...$(RESET)"
	docker-compose -f $(DOCKER_COMPOSE) down
	@echo "$(GREEN)✓ Services stopped$(RESET)"

docker-logs:
	@echo "$(BLUE)📋 Viewing logs...$(RESET)"
	docker-compose -f $(DOCKER_COMPOSE) logs -f

docker-logs-server:
	@echo "$(BLUE)📋 Viewing server logs...$(RESET)"
	docker-compose -f $(DOCKER_COMPOSE) logs -f voip-server

docker-build:
	@echo "$(BLUE)🐳 Building Docker images...$(RESET)"
	docker-compose -f $(DOCKER_COMPOSE) build

docker-clean:
	@echo "$(RED)🗑 Cleaning Docker containers and volumes...$(RESET)"
	docker-compose -f $(DOCKER_COMPOSE) down -v --remove-orphans
	@echo "$(GREEN)✓ Docker containers and volumes removed$(RESET)"

docker-pull-models:
	@echo "$(BLUE)📥 Pulling LLM models...$(RESET)"
	@sleep 5
	curl -X POST http://localhost:11434/api/pull -d '{"name": "llama3.1:8b"}'
	@echo "$(GREEN)✓ Model pulled$(RESET)"

# =============================================================================
# DEPENDENCY TARGETS
# =============================================================================

install: deps

deps: deps-go deps-dart
	@echo "$(GREEN)✓ All dependencies installed$(RESET)"

deps-go:
	@echo "$(BLUE)📦 Downloading Go dependencies...$(RESET)"
	cd $(GO_DIR) && go mod download
	cd $(GO_DIR) && go mod tidy
	@echo "$(GREEN)✓ Go dependencies ready$(RESET)"

deps-dart:
	@echo "$(BLUE)📦 Getting Dart packages...$(RESET)"
	cd $(DART_DIR) && dart pub get
	@echo "$(GREEN)✓ Dart packages ready$(RESET)"

# =============================================================================
# CODE QUALITY TARGETS
# =============================================================================

lint:
	@echo "$(BLUE)🔍 Running Go linter...$(RESET)"
	@which golangci-lint > /dev/null || (echo "$(RED)golangci-lint not found. Install from https://golangci-lint.run/usage/install/$(RESET)" && exit 1)
	cd $(GO_DIR) && golangci-lint run ./...
	@echo "$(GREEN)✓ Linting complete$(RESET)"

format:
	@echo "$(BLUE)✨ Formatting Go code...$(RESET)"
	cd $(GO_DIR) && go fmt ./...
	@echo "$(BLUE)✨ Formatting Dart code...$(RESET)"
	cd $(DART_DIR) && dart format .
	@echo "$(GREEN)✓ Formatting complete$(RESET)"

# =============================================================================
# CLEAN TARGETS
# =============================================================================

clean:
	@echo "$(RED)🗑 Cleaning build artifacts...$(RESET)"
	@rm -rf $(BUILD_DIR)
	@rm -f $(WEB_OUTPUT)
	@cd $(GO_DIR) && go clean -cache
	@echo "$(GREEN)✓ Build artifacts cleaned$(RESET)"

clean-all: clean docker-clean
	@echo "$(RED)🗑 Deep cleaning...$(RESET)"
	@cd $(GO_DIR) && go clean -modcache
	@cd $(DART_DIR) && rm -rf .dart_tool pubspec.lock
	@echo "$(GREEN)✓ Deep clean complete$(RESET)"

# =============================================================================
# PACKAGE & RELEASE TARGETS
# =============================================================================

package: build
	@echo "$(BLUE)📦 Creating release package...$(RESET)"
	@mkdir -p $(DIST_DIR)/$(PROJECT_NAME)-$(VERSION)
	@cp -r $(GO_DIR)/configs $(DIST_DIR)/$(PROJECT_NAME)-$(VERSION)/
	@cp $(SERVER_BIN) $(DIST_DIR)/$(PROJECT_NAME)-$(VERSION)/
	@cp $(TUI_BIN) $(DIST_DIR)/$(PROJECT_NAME)-$(VERSION)/
	@cp -r $(WEB_DIR) $(DIST_DIR)/$(PROJECT_NAME)-$(VERSION)/web
	@cp docker-compose.yml $(DIST_DIR)/$(PROJECT_NAME)-$(VERSION)/
	@cp README.md $(DIST_DIR)/$(PROJECT_NAME)-$(VERSION)/
	@cp LICENSE $(DIST_DIR)/$(PROJECT_NAME)-$(VERSION)/
	@cd $(DIST_DIR) && tar -czf $(PROJECT_NAME)-$(VERSION).tar.gz $(PROJECT_NAME)-$(VERSION)
	@echo "$(GREEN)✓ Package created: $(DIST_DIR)/$(PROJECT_NAME)-$(VERSION).tar.gz$(RESET)"

release: clean build
	@echo "$(BLUE)🔨 Building release binaries...$(RESET)"
	@mkdir -p $(DIST_DIR)
	
	# Linux AMD64
	@echo "  → linux/amd64"
	@cd $(GO_DIR) && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		go build -mod=mod -o ../$(DIST_DIR)/$(PROJECT_NAME)-linux-amd64 \
		-ldflags="-s -w -X main.Version=$(VERSION)" ./cmd/server
	@cd $(GO_DIR) && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		go build -mod=mod -o ../$(DIST_DIR)/tui-agent-linux-amd64 \
		-ldflags="-s -w" ./cmd/tui-agent
	
	# Darwin AMD64
	@echo "  → darwin/amd64"
	@cd $(GO_DIR) && GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 \
		go build -mod=mod -o ../$(DIST_DIR)/$(PROJECT_NAME)-darwin-amd64 \
		-ldflags="-s -w -X main.Version=$(_VERSION)" ./cmd/server
	@cd $(GO_DIR) && GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 \
		go build -mod=mod -o ../$(DIST_DIR)/tui-agent-darwin-amd64 \
		-ldflags="-s -w" ./cmd/tui-agent
	
	# Windows AMD64
	@echo "  → windows/amd64"
	@cd $(GO_DIR) && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
		go build -mod=mod -o ../$(DIST_DIR)/$(PROJECT_NAME)-windows-amd64.exe \
		-ldflags="-s -w -X main.Version=$(VERSION)" ./cmd/server
	@cd $(GO_DIR) && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
		go build -mod=mod -o ../$(DIST_DIR)/tui-agent-windows-amd64.exe \
		-ldflags="-s -w" ./cmd/tui-agent
	
	@echo "$(GREEN)✓ Release binaries created in $(DIST_DIR)/$(RESET)"

# =============================================================================
# HEALTH CHECK TARGETS
# =============================================================================

health:
	@echo "$(BLUE)🏥 Health checks...$(RESET)"
	@echo "  Checking Go installation..."
	@go version || (echo "$(RED)Go not found$(RESET)" && exit 1)
	@echo "  Checking Docker..."
	@docker --version || (echo "$(RED)Docker not found$(RESET)" && exit 1)
	@echo "  Checking Dart..."
	@dart --version || (echo "$(RED)Dart not found$(RESET)" && exit 1)
	@echo "$(GREEN)✓ All dependencies found$(RESET)"

status:
	@echo "$(BLUE)📊 Service Status:$(RESET)"
	@echo "  Redis: $$(docker ps -q -f name=voip-redis 2>/dev/null | wc -l | tr -d ' ') containers"
	@echo "  Ollama: $$(docker ps -q -f name=voip-ollama 2>/dev/null | wc -l | tr -d ' ') containers"
	@echo "  Whisper: $$(docker ps -q -f name=voip-whisper 2>/dev/null | wc -l | tr -d ' ') containers"
	@echo "  Piper: $$(docker ps -q -f name=voip-piper 2>/dev/null | wc -l | tr -d ' ') containers"
	@echo "  VoIP Server: $$(docker ps -q -f name=voip-server 2>/dev/null | wc -l | tr -d ' ') containers"
