# Getting Started

## System Requirements

### Minimum Requirements
- **OS**: Linux (Ubuntu 22.04+ recommended), macOS, or Windows with WSL2
- **CPU**: 4 cores (8+ recommended for concurrent calls)
- **RAM**: 8GB (16GB+ recommended with LLM)
- **Storage**: 20GB free space
- **Network**: Ports 5060/udp, 8080/tcp, 6379/tcp available

### Recommended for Production
- **CPU**: 8+ cores (for 50+ concurrent calls)
- **RAM**: 32GB (with GPU acceleration)
- **GPU**: NVIDIA with CUDA support (for faster Whisper inference)
- **Storage**: SSD with 100GB+ free space

## Installation

### Option 1: Docker Compose (Recommended)

```bash
# Clone the repository
git clone https://github.com/your-org/voip-server.git
cd voip-server

# Install dependencies
make install       # Or: make deps

# Build everything
make build

# Start services
make docker-up     # Starts Redis, Ollama, Whisper, Piper

# Pull LLM model (one-time, ~4GB)
make docker-pull-models
```

### Option 2: Manual Installation

```bash
# 1. Install Go 1.21+
# https://go.dev/doc/install

# 2. Install Dart SDK 3.0+
# https://dart.dev/get-dart

# 3. Download the project
cd /opt
git clone https://github.com/your-org/voip-server.git
cd voip-server

# 4. Get dependencies
make deps

# 5. Build all components
make build

# 6. Start services manually
# Redis
docker run -d -p 6379:6379 redis:7-alpine

# Ollama
docker run -d -p 11434:11434 ollama/ollama:latest
curl -X POST http://localhost:11434/api/pull -d '{"name": "llama3.1:8b"}'

# Whisper STT
docker run -d -p 9090:8000 fedirz/faster-whisper-server:latest-cpu

# Piper TTS
docker run -d -p 5000:10200 rhasspy/wyoming-piper:latest
```

## Quick Start

### Running the Server

```bash
# Option A: Run directly
make run

# Option B: Run in Docker
make docker-up

# Option C: Run with custom config
./build/voip-server -config /path/to/config.yaml
```

### Connecting an Agent

#### Terminal UI (TUI)

```bash
# Build and run
make run-tui

# Or manually
export WS_URL="ws://localhost:8080/ws"
export AGENT_ID="agent_001"
./build/tui-agent
```

**TUI Controls:**
- `F1` — Accept transfer
- `F2` — Reject transfer
- `F3` — Terminate call
- `F10` — Show help
- `ESC` — Quit

#### Web Dashboard

```bash
# Build and serve
make run-web

# Access at: http://localhost:8081
```

### Making a Test Call

```bash
# Using API
curl -X POST http://localhost:8080/api/v1/test/create \
  -d '{"caller_id":"test_caller","caller_number":"+1234567890"}'

# Or using a SIP client like Zoiper
# Server: localhost:5060
# Username: test
```

### Verifying Health

```bash
# Check server health
curl http://localhost:8080/health

# Check all services
make status
```

## Next Steps

- [Configure your system](configuration.md)
- [Learn the TUI Agent](tui-agent.md)
- [Explore the Web Dashboard](web-dashboard.md)
- [Review API Reference](api-reference.md)
