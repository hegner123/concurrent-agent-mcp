# Binary name
binary_name := "hq"
install_path := "/usr/local/bin"

# Build the binary
build:
    @echo "Building {{binary_name}}..."
    @go build -o bin/{{binary_name}} .
    @echo "Built bin/{{binary_name}}"

# Install to system
install: build
    @echo "Installing to {{install_path}}/{{binary_name}}..."
    @cp bin/{{binary_name}} {{install_path}}/{{binary_name}}
    @chmod +x {{install_path}}/{{binary_name}}
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
    @./bin/{{binary_name}}

# Run the server
run: build
    @./bin/{{binary_name}}

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
    @GOOS=darwin GOARCH=amd64 go build -o bin/{{binary_name}}-darwin-amd64 .
    @GOOS=darwin GOARCH=arm64 go build -o bin/{{binary_name}}-darwin-arm64 .
    @GOOS=linux GOARCH=amd64 go build -o bin/{{binary_name}}-linux-amd64 .
    @GOOS=linux GOARCH=arm64 go build -o bin/{{binary_name}}-linux-arm64 .
    @echo "Built all platforms"

# Open database shell
db-shell:
    @sqlite3 ~/.claude/agent-coordination.db

# Reset database (will recreate on next run)
db-reset: clean
    @echo "Database reset (will recreate on next run)"

# Backup database
db-backup:
    @echo "Backing up database..."
    @cp ~/.claude/agent-coordination.db ~/.claude/agent-coordination-backup-$(date +%Y%m%d-%H%M%S).db
    @echo "Backup created"
