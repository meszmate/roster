.PHONY: build run clean test lint install plugins

BINARY_NAME=roster
BUILD_DIR=build
PLUGIN_DIR=plugins

# Build the main binary
build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/roster

# Build with debug symbols
build-debug:
	@mkdir -p $(BUILD_DIR)
	go build -gcflags="all=-N -l" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/roster

# Run the application
run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	go clean

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run linter
lint:
	golangci-lint run

# Install to GOBIN
install:
	go install ./cmd/roster

# Build plugins
plugins:
	@mkdir -p $(BUILD_DIR)/plugins
	@for dir in $(PLUGIN_DIR)/*/; do \
		if [ -f "$$dir/main.go" ]; then \
			name=$$(basename $$dir); \
			echo "Building plugin: $$name"; \
			go build -o $(BUILD_DIR)/plugins/$$name $$dir; \
		fi \
	done

# Generate protobuf files
proto:
	protoc --go_out=. --go-grpc_out=. pkg/plugin/proto/*.proto

# Download dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Run all checks before commit
check: fmt lint test

# Create default config directories
init-config:
	@mkdir -p ~/.config/roster
	@mkdir -p ~/.local/share/roster
	@cp -n configs/config.example.toml ~/.config/roster/config.toml 2>/dev/null || true
	@cp -n configs/accounts.example.toml ~/.config/roster/accounts.toml 2>/dev/null || true
	@echo "Config initialized at ~/.config/roster"

# Show help
help:
	@echo "Available targets:"
	@echo "  build         - Build the roster binary"
	@echo "  build-debug   - Build with debug symbols"
	@echo "  run           - Build and run the application"
	@echo "  clean         - Remove build artifacts"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  lint          - Run linter"
	@echo "  install       - Install to GOBIN"
	@echo "  plugins       - Build all plugins"
	@echo "  proto         - Generate protobuf files"
	@echo "  deps          - Download and tidy dependencies"
	@echo "  fmt           - Format code"
	@echo "  check         - Run fmt, lint, and test"
	@echo "  init-config   - Create default config directories"
