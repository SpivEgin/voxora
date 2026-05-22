# Development

## Building from Source

### Prerequisites

- **Go** 1.21 or later
- **Dart SDK** 3.0 or later
- **Docker** (optional, for services)
- **Make** (optional, for build automation)

### Install Go

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install golang-go

# macOS
brew install go

# Verify
go version
# go version go1.21.0 linux/amd64
```

### Install Dart

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install dart

# macOS
brew install dart

# Verify
dart --version
# Dart SDK version: 3.0.0
```

## Clone and Setup

```bash
# Clone repository
git clone https://github.com/your-org/voip-server.git
cd voip-server

# Install dependencies
make deps
# or
cd voip-server && go mod download
cd ../agent-web && dart pub get
```

## Project Structure

```
voipbox/
├── voip-server/           # Go backend
│   ├── cmd/
│   │   ├── server/        # Main server entry point
│   │   └── tui-agent/     # Terminal UI agent
│   ├── internal/          # Internal packages
│   │   ├── api/           # REST API handlers
│   │   ├── audio/         # Audio pipeline
│   │   ├── config/        # Configuration management
│   │   ├── llm/           # LLM integration
│   │   ├── sip/           # SIP protocol handlers
│   │   ├── state/         # Call state machine
│   │   ├── stt/           # Speech-to-text
│   │   ├── tts/           # Text-to-speech
│   │   ├── transfer/      # Transfer logic
│   │   └── websocket/     # WebSocket handlers
│   ├── configs/
│   │   └── config.yaml    # Default configuration
│   └── Dockerfile
│
├── agent-web/             # Dart web frontend
│   ├── web/
│   │   ├── index.html     # Dashboard HTML
│   │   └── main.dart      # Dart application
│   ├── pubspec.yaml
│   └── build.yaml
│
├── docker-compose.yml      # Docker deployment
├── Makefile               # Build automation
├── README.md
└── LICENSE
```

## Building Components

### Server Binary

```bash
# Using make
make build-server

# Manual
cd voip-server
go build -o ../build/voip-server ./cmd/server
```

**Build Flags:**

| Flag | Description |
|------|-------------|
| `-ldflags="-s -w"` | Strip debug info |
| `-tags=prod` | Production build tags |
| `-race` | Enable race detection |
| `-cover` | Include coverage |

### TUI Agent Binary

```bash
# Using make
make build-tui

# Manual
cd voip-server
go build -o ../build/tui-agent ./cmd/tui-agent
```

### Web Dashboard

```bash
# Using make
make build-web

# Manual
cd agent-web
dart compile js -O2 -o web/main.dart.js web/main.dart
```

## Running Tests

### Unit Tests

```bash
# All tests
make test

# With coverage
cd voip-server
go test -cover ./internal/...

# Specific package
go test ./internal/state -v

# Race detection
go test -race ./internal/...
```

### Coverage Report

```bash
# Generate coverage
go test -coverprofile=coverage.out ./internal/...

# View in browser
go tool cover -html=coverage.out

# Coverage summary
go tool cover -func=coverage.out
```

### Benchmarks

```bash
# Run benchmarks
go test -bench=. ./internal/...

# Memory profiling
go test -bench=. -memprofile=mem.prof ./internal/...

# CPU profiling
go test -bench=. -cpuprofile=cpu.prof ./internal/...
```

## Code Quality

### Linting

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
cd voip-server
golangci-lint run

# Auto-fix
golangci-lint run --fix
```

### Formatting

```bash
# Format Go code
cd voip-server
go fmt ./...

# Format Dart code
cd agent-web
dart format .

# Check formatting
cd voip-server
gofmt -l .
```

### Vet

```bash
# Static analysis
cd voip-server
go vet ./...
```

## Debugging

### Server Debugging

```bash
# Run with debugger
dlv debug ./cmd/server

# Or with delve
go install github.com/go-delve/delve/cmd/dlv@latest
dlv debug ./cmd/server -- -config /path/to/config.yaml
```

**Common Delve Commands:**
```
(dlv) break main.main        # Set breakpoint
(dlv) continue               # Run until breakpoint
(dlv) print variable         # Inspect variable
(dlv) locals                 # Show local variables
(dlv) goroutines             # List goroutines
(dlv) stack                  # Show stack trace
(dlv) quit                   # Exit debugger
```

### Profiling

```bash
# CPU profile
curl http://localhost:8080/debug/pprof/profile > cpu.pprof

# Memory profile
curl http://localhost:8080/debug/pprof/heap > heap.pprof

# Goroutine profile
curl http://localhost:8080/debug/pprof/goroutine > goroutine.pprof

# Analyze
go tool pprof cpu.pprof
```

## Development Workflow

### Adding a New Feature

1. **Create branch:**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Write tests first:**
   ```bash
   # Add tests
   vim voip-server/internal/yourpackage/feature_test.go
   
   # Run tests (should fail - TDD)
   go test ./internal/yourpackage -v
   ```

3. **Implement feature:**
   ```bash
   vim voip-server/internal/yourpackage/feature.go
   ```

4. **Run tests:**
   ```bash
   go test ./internal/yourpackage -v
   ```

5. **Build and test:**
   ```bash
   make build
   make test
   ```

6. **Commit:**
   ```bash
   git add .
   git commit -m "feat: add your feature description"
   ```

### Adding an API Endpoint

1. **Define route in `api/handler.go`:**
   ```go
   v1.Get("/your-endpoint", h.yourHandler)
   ```

2. **Implement handler:**
   ```go
   func (h *Handler) yourHandler(c *fiber.Ctx) error {
       // Implementation
       return c.JSON(fiber.Map{
           "status": "ok",
       })
   }
   ```

3. **Add tests:**
   ```go
   func TestHandler_yourHandler(t *testing.T) {
       // Test implementation
   }
   ```

4. **Update documentation:**
   - Add to `wiki/api-reference.md`
   - Update `README.md`

### Adding a WebSocket Event

1. **Define event type:**
   ```go
   // In websocket/handler.go
   case "your_event":
       c.handleYourEvent(msg.Payload)
   ```

2. **Implement handler:**
   ```go
   func (c *Client) handleYourEvent(payload map[string]interface{}) {
       // Implementation
       c.sendMessage("your_response", map[string]interface{}{
           "data": "value",
       })
   }
   ```

3. **Update Dart client:**
   ```dart
   // In web/main.dart
   case 'your_response':
     handleYourResponse(payload);
     break;
   ```

## Docker Development

### Multi-Stage Build

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o voip-server ./cmd/server

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/voip-server /usr/local/bin/
ENTRYPOINT ["voip-server"]
```

### Hot Reload

```bash
# Install air (live reload)
go install github.com/cosmtrek/air@latest

# Run with hot reload
cd voip-server
air
```

**air.toml:**
```toml
root = "."
build_cmd = "go build -o ./tmp/main ./cmd/server"
run_bin = "./tmp/main"
watch = ["internal"]
ext = ["go"]
```

## Contributing

### Pull Request Process

1. **Fork and clone:**
   ```bash
   git clone https://github.com/your-org/voip-server.git
   cd voip-server
   ```

2. **Create feature branch:**
   ```bash
   git checkout -b feature/description
   ```

3. **Make changes and test:**
   ```bash
   make build
   make test
   ```

4. **Push and create PR:**
   ```bash
   git push origin feature/description
   ```

### Code Style

**Go:**
- Follow Effective Go guidelines
- Use `gofmt` for formatting
- Add godoc comments for exported functions
- Keep functions focused and small

**Dart:**
- Follow Dart style guide
- Use `dart format` for formatting
- Add types to public APIs
- Prefer final/const over var

### Commit Messages

Follow conventional commits:

```
feat: add new transfer condition
docs: update API documentation
fix: resolve WebSocket reconnection bug
test: add coverage for state machine
refactor: simplify audio pipeline
perf: improve STT latency
chore: update dependencies
```

### Documentation

When adding features:

1. Update `README.md` if user-facing
2. Add to `wiki/` for detailed documentation
3. Update `CHANGELOG.md` with version
4. Add inline code comments

## Testing

### Integration Testing

```bash
# Start dependencies
docker-compose up -d redis ollama

# Run integration tests
cd voip-server
go test -tags=integration ./test/...

# End-to-end test
curl -X POST http://localhost:8080/api/v1/test/create \
  -d '{"caller_id":"test","caller_number":"+1"}'
```

### Load Testing

```bash
# Install k6
brew install k6

# Run load test
k6 run load-test.js

# Example load-test.js
import http from 'k6/http';

export default function () {
  http.get('http://localhost:8080/health');
}
```

## Release Process

1. **Update version:**
   ```bash
   # Update version in main.go and config.yaml
   ```

2. **Run tests:**
   ```bash
   make test
   ```

3. **Build release:**
   ```bash
   make release
   ```

4. **Create tag:**
   ```bash
   git tag -a v1.0.0 -m "Release version 1.0.0"
   git push origin v1.0.0
   ```

5. **Create release notes:**
   - List new features
   - Document breaking changes
   - Include upgrade instructions
