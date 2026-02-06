.PHONY: build install test clean dev run

# Binary name
BINARY_NAME=hq
INSTALL_PATH=/usr/local/bin

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@go build -o bin/$(BINARY_NAME) .
	@echo "Built bin/$(BINARY_NAME)"

# Install to system
install: build
	@echo "Installing to $(INSTALL_PATH)/$(BINARY_NAME)..."
	@cp bin/$(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)
	@chmod +x $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Installed successfully"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -f ~/.claude/agent-coordination.db
	@rm -f ~/.claude/agent-coordination.db-shm
	@rm -f ~/.claude/agent-coordination.db-wal
	@echo "Cleaned"

# Development mode (with verbose logging)
dev: build
	@echo "Running in development mode..."
	@./bin/$(BINARY_NAME)

# Run the server
run: build
	@./bin/$(BINARY_NAME)

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	@golangci-lint run

# Generate Go files (if needed)
generate:
	@echo "Generating code..."
	@go generate ./...

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@GOOS=darwin GOARCH=amd64 go build -o bin/$(BINARY_NAME)-darwin-amd64 .
	@GOOS=darwin GOARCH=arm64 go build -o bin/$(BINARY_NAME)-darwin-arm64 .
	@GOOS=linux GOARCH=amd64 go build -o bin/$(BINARY_NAME)-linux-amd64 .
	@GOOS=linux GOARCH=arm64 go build -o bin/$(BINARY_NAME)-linux-arm64 .
	@echo "Built all platforms"

# Database helpers
db-shell:
	@sqlite3 ~/.claude/agent-coordination.db

db-reset: clean
	@echo "Database reset (will recreate on next run)"

db-backup:
	@echo "Backing up database..."
	@cp ~/.claude/agent-coordination.db ~/.claude/agent-coordination-backup-$(shell date +%Y%m%d-%H%M%S).db
	@echo "Backup created"
