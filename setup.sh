#!/bin/bash

# ==========================================
# Go-Mesh-Hub Bootstrap Script
# ==========================================
# This script automates the installation of Go
# and the compilation of the binaries.
# ==========================================

set -e # Exit immediately if a command exits with a non-zero status

# --- Configuration ---
GO_VERSION="1.25.5"
INSTALL_DIR="/usr/local"
PROJECT_DIR=$(pwd)

# --- Colors for UI ---
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_err() { echo -e "${RED}[ERROR]${NC} $1"; }

# 1. Detect Architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        GO_ARCH="amd64"
        ;;
    aarch64)
        GO_ARCH="arm64"
        ;;
    armv7l)
        GO_ARCH="armv6l"
        ;;
    *)
        log_err "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

log_info "Detected Architecture: $ARCH ($GO_ARCH)"

# 2. Check if Go is installed
if ! command -v go &> /dev/null; then
    log_info "Go not found. Installing Go ${GO_VERSION}..."
    
    # Download
    wget -q "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" -O go.tar.gz
    
    # Remove old installation
    if [ -d "$INSTALL_DIR/go" ]; then
        log_info "Removing old Go installation..."
        sudo rm -rf "$INSTALL_DIR/go"
    fi

    # Extract (Requires Sudo)
    log_info "Extracting to $INSTALL_DIR (requires sudo)..."
    sudo tar -C "$INSTALL_DIR" -xzf go.tar.gz
    rm go.tar.gz

    # Setup Path temporarily for this session
    export PATH=$PATH:$INSTALL_DIR/go/bin
    
    log_success "Go installed successfully!"
    
    # Remind user to update path permanently
    echo ""
    echo "⚠️  NOTE: To use 'go' in the future, add this to your ~/.bashrc:"
    echo "   export PATH=\$PATH:$INSTALL_DIR/go/bin"
    echo ""
else
    log_info "Go is already installed: $(go version)"
fi

# 3. Initialize Module (if needed)
if [ ! -f "go.mod" ]; then
    log_info "Initializing Go Module..."
    go mod init go-mesh-hub
    go mod tidy
else
    log_info "Downloading dependencies..."
    go mod download
fi

# 4. Build Binaries
log_info "Building Hub (Server)..."
go build -o bin/hub cmd/hub/main.go

log_info "Building Agent (Client)..."
go build -o bin/agent cmd/agent/main.go

# 5. Finish
echo ""
echo "=========================================="
log_success "Build Complete!"
echo "=========================================="
echo "Find your binaries in the 'bin/' folder:"
ls -lh bin/
echo ""
echo "👉 To run the Server: sudo ./bin/hub -tun-ip 10.0.0.1 ..."
echo "👉 To run the Client: sudo ./bin/agent -hub-ip X.X.X.X ..."
echo ""