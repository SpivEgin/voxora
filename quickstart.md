# VoIP Server Quick Start Guide

Get your self-hosted VoIP system with LLM voice routing up and running in minutes.

## Prerequisites

- **Docker & Docker Compose** (v2.0+)
- **Git**
- 4GB+ RAM (8GB recommended for LLM)
- Ports available: 5060/udp (SIP), 8080 (HTTP/WebSocket), 6379 (Redis)

## Option 1: Quick Start with Docker (Recommended)

### Step 1: Clone and Start Services

```bash
cd voipbox

# Start all services in detached mode
docker-compose up -d

# Wait for services to initialize (30-60 seconds)
sleep 30
```

### Step 2: Pull the LLM Model

```bash
# Monitor model download (one-time setup, ~4GB)
docker-compose logs -f ollama-pull

# Or manually pull if needed:
curl -X POST http://localhost:11434/api/pull \
  -d '{"name": "llama3.1:8b"}'
```

### Step 3: Verify All Services

```bash
# Check all services are running
docker-compose ps

# Expected output:
# voip-redis      running
# voip-ollama     running
# voip-whisper    running
# voip-piper      running
# voip-server     running
```

### Step 4: Test Health Endpoints

```bash
# Test VoIP Server
curl http://localhost:8080/health
# Expected: {"status":"healthy","time":...}

# Test Ollama
curl http://localhost:11434/api/tags
# Expected: List of models including llama3.1:8b
```

## Option 2: Manual Setup (Development)

### Step 1: Start Infrastructure Services

```bash
# Terminal 1: Redis
docker run -d -p 6379:6379 --name voip-redis redis:7-alpine

# Terminal 2: Ollama
docker run -d -p 11434:11434 --name voip-ollama ollama/ollama:latest

# Pull LLM model
curl -X POST http://localhost:11434/api/pull \
  -d '{"name": "llama3.1:8b"}'

# Terminal 3: Whisper STT
docker run -d -p 9090:8000 \
  -e WHISPER_MODEL=base \
  fedirz/faster-whisper-server:latest-cpu

# Terminal 4: Piper TTS
docker run -d -p 5000:10200 \
  rhasspy/wyoming-piper:latest \
  --voice en_US-lessac-medium
```

### Step 2: Configure and Run Go Server

```bash
cd voip-server

# Install dependencies
go mod tidy

# Run the server
go run cmd/server/main.go

# Server should start on:
# - SIP: 0.0.0.0:5060
# - HTTP/WebSocket: 0.0.0.0:8080
```

## Connect an Agent Interface

### Terminal UI Agent (CLI)

```bash
# In a new terminal
cd voip-server

# Set connection details
export WS_URL="ws://localhost:8080/ws"
export AGENT_ID="agent_001"

# Run the TUI agent
go run cmd/tui-agent/main.go
```

**Controls:**
- `F1` - Accept transfer request
- `F2` - Reject transfer request  
- `F3` - Terminate selected call
- `F10` - Show help
- `ESC` - Quit
- `Tab/Arrow` - Navigate between panels
- `Enter` - Select call

### Web Dashboard (Browser)

```bash
cd agent-web

# Get dependencies
dart pub get

# Build to JavaScript
dart compile js -O2 -o web/main.dart.js web/main.dart

# Serve (or use any static server)
python3 -m http.server 8081 --directory web

# Open in browser
open http://localhost:8081/index.html
```

## Making Test Calls

### Option A: Using SIP Client (e.g., Zoiper, Linphone)

```
Server: localhost:5060
Username: test
Password: (none in dev mode)
```

### Option B: Using Webhook (Twilio Compatible)

```bash
# Configure webhook URL in your provider
# Point to: http://your-server:8080/webhooks/twilio

# Expected TwiML Response:
# <?xml version="1.0" encoding="UTF-8"?>
# <Response>
#   <Connect>
#     <Stream url="ws://localhost:8080/ws" />
#   </Connect>
# </Response>
```

### Option C: REST API Test

```bash
# List active calls
curl http://localhost:8080/api/v1/calls | jq

# Create test session (for development)
curl -X POST http://localhost:8080/api/v1/test/create \
  -d '{"caller_id":"test_caller","caller_number":"+1234567890"}'
```

## Verification Checklist

- [ ] All Docker containers running (`docker-compose ps`)
- [ ] Health endpoint responds (`curl localhost:8080/health`)
- [ ] Ollama has model (`curl localhost:11434/api/tags`)
- [ ] Agent can connect (TUI or Web shows "Connected")
- [ ] Test call appears in call list
- [ ] Transcript updates in real-time
- [ ] Transfer logic works (try saying "I want to speak to a human")

## Common Issues & Fixes

### Issue: Port 5060 already in use

```bash
# Find process using port 5060
sudo lsof -i :5060

# Kill it or change SIP port in config.yaml
```

### Issue: Ollama model download fails

```bash
# Check Ollama logs
docker-compose logs ollama

# Manual download
curl -X POST http://localhost:11434/api/pull \
  -H "Content-Type: application/json" \
  -d '{"name": "llama3.1:8b"}'
```

### Issue: Redis connection refused

```bash
# Verify Redis is running
docker ps | grep redis

# Test connection
redis-cli ping
# Expected: PONG
```

### Issue: STT service not responding

```bash
# Check Whisper health
curl http://localhost:9090/health

# Fallback to Vosk if Whisper fails
# Edit configs/config.yaml:
# stt:
#   engine: "vosk"
```

### Issue: WebSocket connection fails

```bash
# Check if server is listening
netstat -tlnp | grep 8080

# Verify WebSocket endpoint
curl -i -N \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Host: localhost:8080" \
  -H "Origin: http://localhost:8080" \
  http://localhost:8080/ws
```

## Configuration Quick Reference

Edit `voip-server/configs/config.yaml`:

```yaml
# Change LLM model
llm:
  ollama:
    model: "llama3.1:8b"  # or "mistral:7b", "phi3:mini"

# Adjust transfer conditions
transfer:
  conditions:
    confidence_threshold: 0.6  # Lower = more transfers
    max_llm_turns: 10          # Max AI interactions
    max_call_duration: 600     # 10 minutes

# Enable/disable services
server:
  sip:
    enabled: true
  websocket:
    enabled: true
```

## Viewing Logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f voip-server
docker-compose logs -f ollama
docker-compose logs -f whisper

# Go server (manual mode)
go run cmd/server/main.go 2>&1 | tee server.log
```

## Performance Tuning

### For 50+ Concurrent Calls:

1. **Use GPU for Whisper** (change image tag in docker-compose.yml):
   ```yaml
   image: fedirz/faster-whisper-server:latest-cuda
   ```

2. **Use GPU for Ollama**:
   ```yaml
   deploy:
     resources:
       reservations:
         devices:
           - driver: nvidia
             count: 1
             capabilities: [gpu]
   ```

3. **Increase Go server workers**:
   Edit configs/config.yaml:
   ```yaml
   server:
     workers: 100
   ```

## Next Steps

1. **Production Setup**:
   - Enable TLS/SRTP in config.yaml
   - Configure Redis persistence
   - Set up monitoring (Prometheus/Grafana)
   - Configure SIP trunk provider

2. **Customize**:
   - Modify transfer conditions in config.yaml
   - Customize LLM system prompt
   - Add custom STT/TTS models
   - Implement webhook integrations

3. **Integrate**:
   - Connect to your SIP provider
   - Set up agent authentication
   - Configure call recording
   - Add CRM integration

## Getting Help

- **Documentation**: See `README.md` for full details
- **API Reference**: Visit `http://localhost:8080/api/v1/` endpoints
- **Logs**: Check `docker-compose logs` for errors
- **Issues**: Review troubleshooting section above

---

**Ready to go!** Your VoIP server should now be handling calls with AI-powered routing. 🎉
