# VoIP Server with LLM Voice Routing

A comprehensive VoIP server with self-hosted LLM voice routing, STT, TTS, and agent interfaces.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           VoIP Server System                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────┐    ┌──────────────────┐    ┌─────────────────────┐ │
│  │   SIP Handler   │◄──►│  Audio Pipeline  │◄──►│    STT Engine     │ │
│  │   (sipgo)       │    │   (WebSocket)    │    │  (Whisper/Vosk)   │ │
│  └────────┬────────┘    └──────────────────┘    └─────────────────────┘ │
│           │                                                             │
│           ▼                                                             │
│  ┌─────────────────┐    ┌──────────────────┐    ┌─────────────────────┐ │
│  │   Call State    │◄──►│  LLM Inference   │◄──►│    TTS Engine       │ │
│  │    Machine      │    │ (Ollama/vLLM)    │    │ (Piper/Coqui/Kokoro)│ │
│  └────────┬────────┘    └──────────────────┘    └─────────────────────┘ │
│           │                                                             │
│           ▼                                                             │
│  ┌─────────────────┐    ┌──────────────────┐    ┌─────────────────────┐ │
│  │ Transfer Logic  │◄──►│  Session Store   │◄──►│   Agent Dashboard   │ │
│  │  (SIP REFER)    │    │    (Redis)       │    │  (CLI/TUI + Web)  │ │
│  └─────────────────┘    └──────────────────┘    └─────────────────────┘ │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Features

### Core Features
- **SIP Protocol**: Full SIP server with INVITE, ACK, BYE, CANCEL support
- **WebSocket Audio**: Real-time bidirectional audio streaming
- **Call State Machine**: INCOMING → LLM_ROUTING → LIVE_AGENT/TERMINATED
- **Transfer Logic**: Automatic and manual call transfers with configurable rules

### Speech Processing
- **STT**: faster-whisper (primary) + Vosk (fallback)
- **LLM**: Ollama/vLLM/LocalAI support
- **TTS**: Piper (fast) + Coqui (quality) + Kokoro

### Agent Interfaces
- **CLI/TUI**: Terminal-based agent interface using tview
- **Web Dashboard**: Pure Dart Web (no Flutter) browser interface

## Quick Start

### Prerequisites
- Docker and Docker Compose
- Go 1.21+ (for development)
- Dart SDK (for web agent)

### Running with Docker Compose

```bash
# Clone the repository
cd voipbox

# Start all services
docker-compose up -d

# Wait for Ollama to pull the model
docker-compose logs -f ollama-pull

# View logs
docker-compose logs -f voip-server
```

### Running Locally

```bash
# Start Redis
docker run -d -p 6379:6379 redis:7-alpine

# Start Ollama
docker run -d -p 11434:11434 ollama/ollama:latest

# Pull LLM model
curl -X POST http://localhost:11434/api/pull -d '{"name": "llama3.1:8b"}'

# Run the server
cd voip-server
go run cmd/server/main.go
```

### Running the CLI/TUI Agent

```bash
cd voip-server

# Set environment variables
export WS_URL="ws://localhost:8080/ws"
export AGENT_ID="agent_001"

# Run the TUI agent
go run cmd/tui-agent/main.go
```

Keyboard shortcuts:
- **F1**: Accept transfer
- **F2**: Reject transfer  
- **F3**: Terminate call
- **F10**: Show help
- **Esc**: Quit

### Building and Running Web Agent

```bash
cd agent-web

# Get dependencies
dart pub get

# Build for web
dart compile js -O2 -o web/main.dart.js web/main.dart

# Or serve directly (requires dart2js)
dart run build_runner serve web:8081

# Open in browser
open http://localhost:8081/web/index.html
```

## API Endpoints

### REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| GET | `/ready` | Readiness check |
| GET | `/api/v1/calls` | List all calls |
| GET | `/api/v1/calls/:id` | Get call details |
| POST | `/api/v1/calls/:id/terminate` | Terminate call |
| POST | `/api/v1/calls/:id/transfer` | Transfer call |
| GET | `/api/v1/calls/:id/transcript` | Get transcript |
| GET | `/api/v1/agents` | List agents |
| GET | `/api/v1/system/stats` | System stats |
| GET | `/api/v1/system/health` | Health status |
| GET | `/api/v1/transfer/targets` | Transfer targets |

### WebSocket Events

**Client to Server:**
- `audio_data`: Send audio data
- `join_call`: Join a call
- `leave_call`: Leave a call
- `agent_login`: Login as agent
- `agent_logout`: Logout as agent
- `accept_transfer`: Accept transfer request
- `reject_transfer`: Reject transfer request
- `subscribe_calls`: Subscribe to call events

**Server to Client:**
- `active_calls`: List of active calls
- `call_started`: New call started
- `call_ended`: Call ended
- `transcript_update`: Transcript update
- `transfer_requested`: Transfer requested
- `state_change`: Call state changed
- `error`: Error message

## Configuration

See `voip-server/configs/config.yaml` for full configuration options.

### Key Configuration Options

```yaml
server:
  sip:
    enabled: true
    host: "0.0.0.0"
    port: 5060
    
  websocket:
    enabled: true
    port: 8080
    
  rest:
    enabled: true
    port: 8080

stt:
  engine: "whisper"  # or "vosk"
  whisper:
    host: "localhost"
    port: 9090
    model: "base"

llm:
  backend: "ollama"
  ollama:
    host: "localhost"
    port: 11434
    model: "llama3.1:8b"

tts:
  engine: "piper"  # or "coqui", "kokoro"
  piper:
    host: "localhost"
    port: 5000

transfer:
  enabled: true
  conditions:
    caller_requests_human: true
    confidence_threshold: 0.6
    intent_triggers: ["complaint", "refund", "emergency"]
    max_llm_turns: 10
```

## Development

### Running Tests

```bash
cd voip-server
go test ./internal/... -v

# Run with coverage
go test ./internal/... -cover

# Run specific package tests
go test ./internal/state -v
go test ./internal/transfer -v
```

### Project Structure

```
voipbox/
├── voip-server/
│   ├── cmd/
│   │   ├── server/          # Main server
│   │   └── tui-agent/       # Terminal UI agent
│   ├── internal/
│   │   ├── config/          # Configuration management
│   │   ├── sip/             # SIP protocol handlers
│   │   ├── audio/           # Audio pipeline
│   │   ├── stt/             # Speech-to-text
│   │   ├── llm/             # LLM integration
│   │   ├── tts/             # Text-to-speech
│   │   ├── state/           # Call state management
│   │   ├── transfer/        # Transfer logic
│   │   ├── api/             # REST API
│   │   └── websocket/       # WebSocket handlers
│   ├── configs/
│   │   └── config.yaml
│   └── Dockerfile
├── agent-web/
│   ├── web/
│   │   ├── index.html
│   │   └── main.dart        # Dart web app
│   └── pubspec.yaml
├── docker-compose.yml
└── README.md
```

## Transfer Conditions

The system automatically transfers calls based on configurable conditions:

1. **Caller Requests Human**: Detected via keyword matching
2. **Low LLM Confidence**: Below threshold (default 0.6)
3. **Intent Triggers**: Specific intents like "complaint", "refund", "emergency"
4. **LLM Request**: LLM explicitly requests transfer
5. **Max Turns Exceeded**: Default 10 LLM interactions
6. **Max Duration**: Default 600 seconds
7. **Abusive Language**: Optional profanity detection

## Security

- TLS/SRTP support for encrypted signaling and media
- Rate limiting on API endpoints
- JWT-based agent authentication
- CORS configuration for web interface

## Performance

- Concurrent call handling (50+ calls target)
- Non-blocking audio processing with Go goroutines
- Redis for distributed session state
- Configurable timeouts and resource limits

## Troubleshooting

### Check service health
```bash
# Check all services
docker-compose ps

# View logs
docker-compose logs -f [service-name]

# Test individual services
curl http://localhost:11434/api/tags  # Ollama
curl http://localhost:9090/health     # Whisper
curl http://localhost:8080/health      # VoIP Server
```

### Common Issues

**Ollama model not found:**
```bash
docker-compose logs ollama-pull
# Or manually:
curl -X POST http://localhost:11434/api/pull -d '{"name": "llama3.1:8b"}'
```

**Redis connection failed:**
```bash
# Verify Redis is running
docker-compose up -d redis
```

**SIP port binding:**
```bash
# Check if port 5060 is in use
sudo lsof -i :5060
```

## License

GNU General Public License v3.0 (GPL-3.0) - See [LICENSE](LICENSE) file for details.

This program is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for more details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## Support

For issues and questions:
- Open an issue on GitHub
- Check the logs: `docker-compose logs -f voip-server`
- Review the documentation in this README

---

Built with ❤️ using Go, Dart, and self-hosted AI.
