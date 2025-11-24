# Unified Access Gateway Makefile

# Variables
BINARY_NAME=uag
BINARY_WINDOWS=$(BINARY_NAME).exe
BINARY_LINUX=$(BINARY_NAME)
GO=go
GOFLAGS=-v
LDFLAGS=-ldflags "-s -w"

# eBPF build variables
CLANG=clang
CFLAGS=-O2 -g -Wall -Werror -target bpf -D__TARGET_ARCH_x86_64
BPF_INCLUDE=$(PKG_EBPF_DIR)/include

# Directories
CMD_DIR=./cmd/gateway
PKG_EBPF_DIR=./pkg/ebpf

.PHONY: all build build-linux build-windows clean test generate-ebpf install-deps help

# Default target
all: build

## build: Build the gateway binary for current platform
build:
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_NAME) $(CMD_DIR)
	@echo "Build complete: $(BINARY_NAME)"

## build-linux: Build for Linux
build-linux:
	@echo "Building for Linux..."
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_LINUX) $(CMD_DIR)
	@echo "Build complete: $(BINARY_LINUX)"

## build-windows: Build for Windows
build-windows:
	@echo "Building for Windows..."
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_WINDOWS) $(CMD_DIR)
	@echo "Build complete: $(BINARY_WINDOWS)"

## generate-ebpf: Generate eBPF Go bindings from C code
generate-ebpf:
	@echo "Generating eBPF bindings..."
	@if ! command -v bpf2go > /dev/null; then \
		echo "Error: bpf2go not found. Installing..."; \
		$(GO) install github.com/cilium/ebpf/cmd/bpf2go@latest; \
	fi
	@if ! command -v $(CLANG) > /dev/null; then \
		echo "Error: clang not found. Please install: apt-get install clang llvm"; \
		exit 1; \
	fi
	@echo "Compiling SockMap program..."
	cd $(PKG_EBPF_DIR) && $(GO) generate ./sockmap.go
	@echo "Compiling XDP program..."
	cd $(PKG_EBPF_DIR) && $(GO) generate ./xdp.go
	@echo "eBPF bindings generated successfully"

## install-deps: Install build dependencies (Linux only)
install-deps:
	@echo "Installing dependencies..."
	@echo "Note: This requires root privileges and apt package manager (Debian/Ubuntu)"
	@echo "For other distributions, manually install: clang, llvm, libbpf-dev, linux-headers"
	@read -p "Continue? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		sudo apt-get update && \
		sudo apt-get install -y clang llvm libbpf-dev linux-headers-$$(uname -r) && \
		$(GO) install github.com/cilium/ebpf/cmd/bpf2go@latest; \
	fi
	@echo "Dependencies installed"

## test: Run unit tests
test:
	@echo "Running tests..."
	$(GO) test -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	@echo "Tests complete"

## test-coverage: Run tests with coverage report
test-coverage: test
	@echo "Generating coverage report..."
	$(GO) tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report: coverage.html"

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME) $(BINARY_WINDOWS) $(BINARY_LINUX)
	rm -f coverage.txt coverage.html
	rm -f $(PKG_EBPF_DIR)/bpf_*.go $(PKG_EBPF_DIR)/bpf_*.o
	@echo "Clean complete"

## fmt: Format Go code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...
	@echo "Format complete"

## lint: Run linters (requires golangci-lint)
lint:
	@echo "Running linters..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Install: https://golangci-lint.run/usage/install/"; \
	fi

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t skynet/unified-access-gateway:latest .
	@echo "Docker build complete"

## docker-run: Run Docker container
docker-run:
	@echo "Running Docker container..."
	docker run -p 8080:8080 -p 9090:9090 \
		-e HTTP_BACKEND_URL="http://host.docker.internal:5000" \
		-e TCP_BACKEND_ADDR="host.docker.internal:6000" \
		skynet/unified-access-gateway:latest

## k8s-deploy: Deploy to Kubernetes
k8s-deploy:
	@echo "Deploying to Kubernetes..."
	kubectl apply -f deploy/deployment.yaml
	kubectl apply -f deploy/service.yaml
	kubectl apply -f deploy/hpa.yaml
	@echo "Deployment complete"

## k8s-delete: Delete from Kubernetes
k8s-delete:
	@echo "Deleting from Kubernetes..."
	kubectl delete -f deploy/hpa.yaml
	kubectl delete -f deploy/service.yaml
	kubectl delete -f deploy/deployment.yaml
	@echo "Deletion complete"

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

.DEFAULT_GOAL := help

