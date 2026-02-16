# Oculo — The Glass Box for AI Agents
# Build system for daemon, TUI, CLI, and SDK

.PHONY: all build daemon tui cli test test-go test-python bench clean install proto fmt lint

# Build configuration
VERSION := 0.1.0
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || echo "unknown")
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME)"
GO_TAGS := -tags fts5

# Output directory
BIN_DIR := bin

# Default target: build everything
all: build

# ─────────────────────────────────────────────────────────
# Build targets
# ─────────────────────────────────────────────────────────

build: daemon tui cli

daemon:
	@echo "🔨 Building oculo-daemon..."
	@mkdir -p $(BIN_DIR)
	go build $(GO_TAGS) $(LDFLAGS) -o $(BIN_DIR)/oculo-daemon ./cmd/oculo-daemon

tui:
	@echo "🔨 Building oculo-tui..."
	@mkdir -p $(BIN_DIR)
	go build $(GO_TAGS) $(LDFLAGS) -o $(BIN_DIR)/oculo-tui ./cmd/oculo-tui

cli:
	@echo "🔨 Building oculo..."
	@mkdir -p $(BIN_DIR)
	go build $(GO_TAGS) $(LDFLAGS) -o $(BIN_DIR)/oculo ./cmd/oculo

# ─────────────────────────────────────────────────────────
# Testing
# ─────────────────────────────────────────────────────────

test: test-go test-python

test-go:
	@echo "🧪 Running Go tests..."
	go test $(GO_TAGS) -v -count=1 ./...

test-python:
	@echo "🐍 Running Python tests..."
	cd sdk/python && python -m pytest tests/ -v 2>/dev/null || echo "Python tests skipped (pytest not installed)"

bench:
	@echo "⚡ Running benchmarks..."
	go test $(GO_TAGS) -bench=. -benchmem ./internal/database/...

# ─────────────────────────────────────────────────────────
# Code quality
# ─────────────────────────────────────────────────────────

fmt:
	@echo "✨ Formatting Go code..."
	gofmt -s -w .
	goimports -w . 2>/dev/null || true

lint:
	@echo "🔍 Linting Go code..."
	go vet $(GO_TAGS) ./...
	golangci-lint run 2>/dev/null || echo "golangci-lint not installed, skipping"

# ─────────────────────────────────────────────────────────
# Installation
# ─────────────────────────────────────────────────────────

install: build
	@echo "📦 Installing Oculo binaries..."
	go install $(GO_TAGS) ./cmd/oculo-daemon
	go install $(GO_TAGS) ./cmd/oculo-tui
	go install $(GO_TAGS) ./cmd/oculo

install-sdk:
	@echo "🐍 Installing Python SDK..."
	cd sdk/python && pip install -e .

# ─────────────────────────────────────────────────────────
# Protocol Buffers (optional — for regenerating from .proto)
# ─────────────────────────────────────────────────────────

proto:
	@echo "📝 Generating protobuf code..."
	protoc --go_out=. --go_opt=paths=source_relative \
		internal/protocol/trace.proto

# ─────────────────────────────────────────────────────────
# Development helpers
# ─────────────────────────────────────────────────────────

dev-daemon: daemon
	@echo "🚀 Starting daemon in dev mode..."
	./$(BIN_DIR)/oculo-daemon --db ./dev.db

dev-tui: tui
	@echo "🖥️  Starting TUI in dev mode..."
	./$(BIN_DIR)/oculo-tui --db ./dev.db

# ─────────────────────────────────────────────────────────
# Cleanup
# ─────────────────────────────────────────────────────────

clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -rf $(BIN_DIR)
	rm -f dev.db dev.db-wal dev.db-shm
	go clean -cache
