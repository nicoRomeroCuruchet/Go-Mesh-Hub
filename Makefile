# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
BINARY_DIR=bin
HUB_BINARY_NAME=hub
AGENT_BINARY_NAME=agent

.PHONY: all build clean test docker

all: build

build:
	@echo "🚀 Building binaries..."
	mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(BINARY_DIR)/$(HUB_BINARY_NAME) cmd/hub/main.go
	$(GOBUILD) -o $(BINARY_DIR)/$(AGENT_BINARY_NAME) cmd/agent/main.go
	@echo "✅ Build complete! Binaries are in $(BINARY_DIR)/"

clean:
	@echo "🧹 Cleaning..."
	$(GOCLEAN)
	rm -rf $(BINARY_DIR)

test:
	@echo "🧪 Running tests..."
	$(GOTEST) -v ./...

deps:
	@echo "📦 Downloading dependencies..."
	$(GOCMD) mod download

# Cross-compilation helper (Build for ARM64 on an AMD64 machine)
build-arm64:
	@echo "🚀 Building for ARM64 (Jetson/RPi)..."
	mkdir -p $(BINARY_DIR)
	GOOS=linux GOARCH=arm64 $(GOBUILD) -o $(BINARY_DIR)/$(HUB_BINARY_NAME)-arm64 cmd/hub/main.go
	GOOS=linux GOARCH=arm64 $(GOBUILD) -o $(BINARY_DIR)/$(AGENT_BINARY_NAME)-arm64 cmd/agent/main.go