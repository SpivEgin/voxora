# Troubleshooting

## Common Issues

### Server Won't Start

**Error:** `Failed to initialize logger: open sink "": no such file or directory`

**Fix:** Update `config.yaml` logging output:
```yaml
logging:
  output: "stdout"  # or a valid file path
```

---

**Error:** `dial tcp :6379: connect: connection refused`

**Fix:** Redis is not running:
```bash
# Start Redis
docker-compose up -d redis

# Or check if Redis is listening
redis-cli ping
# Should return: PONG
```

---

**Error:** `SIP server error: listen udp :5060: bind: address already in use`

**Fix:** Port 5060 is already in use:
```bash
# Find process using port
sudo lsof -i :5060

# Kill process or change port in config.yaml
server:
  sip:
    port: 5062  # Use different port
```

---

### LLM Not Responding

**Error:** `LLM completion failed: Post "http://localhost:11434/api/generate": connection refused`

**Fix:** Ollama is not running or model not pulled:
```bash
# Start Ollama
docker-compose up -d ollama

# Wait 10 seconds, then pull model
curl -X POST http://localhost:11434/api/pull \
  -d '{"name": "llama3.1:8b"}'

# Verify model is available
curl http://localhost:11434/api/tags
```

---

**Error:** `LLM response timeout`

**Fix:** Increase timeout in config:
```yaml
llm:
  ollama:
    timeout: 60  # Increase from default 30s
```

Or use a faster model:
```bash
# Pull smaller, faster model
curl -X POST http://localhost:11434/api/pull \
  -d '{"name": "phi3:mini"}'
```

---

### STT Not Working

**Error:** `STT transcription failed: whisper API error: 500`

**Fix:** Whisper model not loaded:
```bash
# Restart Whisper
docker-compose restart whisper

# Check Whisper logs
docker-compose logs -f whisper

# Test Whisper directly
curl http://localhost:9090/health
```

---

**Error:** `Vosk connection failed: dial tcp :2700: connect: connection refused`

**Fix:** If using Vosk as fallback:
```bash
# Start Vosk (in docker-compose.yml, use profile: vosk)
docker-compose --profile vosk up -d vosk

# Or switch to Whisper in config.yaml
stt:
  engine: "whisper"
```

---

### TTS Not Generating Audio

**Error:** `TTS synthesis failed: piper API error: 500`

**Fix:** Piper voice not downloaded:
```bash
# Restart Piper to trigger voice download
docker-compose restart piper

# Check Piper logs
docker-compose logs -f piper

# Test Piper
curl -X POST http://localhost:5000/synthesize \
  -H "Content-Type: application/json" \
  -d '{"text": "Hello world"}'
```

---

### WebSocket Connection Failing

**Error:** `WebSocket connection failed`

**Fix:**
```bash
# 1. Check server is running
curl http://localhost:8080/health

# 2. Verify WebSocket endpoint
curl -i -N \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Host: localhost:8080" \
  http://localhost:8080/ws

# 3. Check CORS settings
# In config.yaml, ensure:
server:
  rest:
    # If accessing from different origin, configure CORS
```

---

### Transfer Not Happening

**Error:** `Transfer conditions not triggering`

**Fix:** Check transfer config:
```yaml
transfer:
  enabled: true
  conditions:
    confidence_threshold: 0.6  # Lower = more sensitive
    max_llm_turns: 10  # Reduce to trigger sooner
    caller_requests_human: true
    intent_triggers:
      - "complaint"
      - "refund"
```

**Debug:** Enable debug logging:
```yaml
logging:
  level: "debug"
```

---

### Agent Can't Connect

**Error:** `Failed to connect to server`

**Fix:**
```bash
# 1. Verify server address
export WS_URL="ws://correct-hostname:8080/ws"

# 2. Check firewall
nc -zv localhost 8080

# 3. Verify agent login format
# WebSocket message should be:
{
  "type": "agent_login",
  "payload": {
    "agent_id": "agent_001"
  }
}
```

---

## Logs

### Viewing Server Logs

```bash
# Docker logs
docker-compose logs -f voip-server

# File logs (if configured)
tail -f /var/log/voip-server.log

# Systemd service
journalctl -u voip-server -f
```

### Log Levels

| Level | Use Case |
|-------|----------|
| debug | Development, tracing issues |
| info | Normal operations |
| warn | Suspicious but non-critical |
| error | Failures requiring action |

### Enabling Debug Logging

```bash
# Temporary (environment variable)
export VOIP_LOGGING_LEVEL=debug
./voip-server

# Permanent (config.yaml)
logging:
  level: "debug"
```

---

## Health Checks

### Server Health

```bash
# Basic health
curl http://localhost:8080/health
# Expected: {"status":"healthy","time":...}

# Ready check
curl http://localhost:8080/ready
# Expected: {"status":"ready",...}
```

### Service Health

```bash
# Redis
redis-cli ping
# Expected: PONG

# Ollama
curl http://localhost:11434/api/tags
# Expected: JSON with model list

# Whisper
curl http://localhost:9090/health
# Expected: 200 OK

# Piper
curl http://localhost:5000/health
# Expected: 200 OK
```

### Component Status

```bash
# Get detailed health
curl http://localhost:8080/api/v1/system/health

# Example response
{
  "status": {
    "sip": "healthy",
    "redis": "healthy",
    "stt": "healthy",
    "tts": "healthy",
    "llm": "unhealthy: connection refused"
  }
}
```

---

## Diagnostic Commands

### System Resources

```bash
# CPU/Memory
docker stats --no-stream

# Disk space
df -h

# Network connections
ss -tlnp | grep '<:8080\|5060'
```

### Redis Diagnostics

```bash
# Connection info
redis-cli info clients
redis-cli info stats

# Active keys
redis-cli keys 'call_session:*' | wc -l

# Memory usage
redis-cli info memory | grep used_memory_human
```

### SIP Debugging

```bash
# Capture SIP traffic
tcpdump -i any -n port 5060 -w sip.pcap

# Analyze with Wireshark/tcpdump
tshark -r sip.pcap -V

# Check SIP registrations
curl http://localhost:8080/api/v1/system/stats
```

### LLM Testing

```bash
# Test Ollama directly
curl -X POST http://localhost:11434/api/generate \
  -d '{
    "model": "llama3.1:8b",
    "prompt": "Hello, how are you?"
  }'

# Test LLM through server
curl -X POST http://localhost:8080/api/v1/test/llm \
  -d '{"prompt": "test"}'
```

---

## Performance Issues

### High CPU Usage

**Symptoms:** Server slow, calls dropping

**Solutions:**
```bash
# 1. Check top processes
top

# 2. Enable GPU for Whisper (if available)
# Edit docker-compose.yml:
whisper:
  image: fedirz/faster-whisper-server:latest-cuda
  deploy:
    resources:
      reservations:
        devices:
          - driver: nvidia
            count: 1
            capabilities: [gpu]

# 3. Use lighter LLM model
# Edit config.yaml:
llm:
  ollama:
    model: "phi3:mini"  # Smaller, faster
```

### High Memory Usage

**Symptoms:** OOM crashes, system swap

**Solutions:**
```bash
# 1. Check memory usage
free -h

# 2. Limit Ollama memory
docker update --memory=8g voip-ollama

# 3. Reduce concurrent calls in config
server:
  sip:
    # Reduce max concurrent
```

### Call Quality Issues

**Symptoms:** Choppy audio, delays

**Solutions:**
```yaml
# Increase buffer size
audio:
  buffer_size: 640  # Increase from 320

# Adjust silence detection
audio:
  silence_detection:
    threshold: -35    # Less sensitive (was -40)
    hangover: 30      # Longer hangover (was 20)
```

---

## Recovery Procedures

### Complete Restart

```bash
# Stop everything
docker-compose down -v

# Clean up
docker system prune -f

# Restart
docker-compose up -d

# Verify
curl http://localhost:8080/health
```

### Database Reset

```bash
# Flush Redis
redis-cli FLUSHDB

# Or persistent clear
docker exec voip-redis redis-cli FLUSHDB
```

### Model Re-download

```bash
# Remove Ollama model
docker exec voip-ollama ollama rm llama3.1:8b

# Re-pull
docker exec voip-ollama ollama pull llama3.1:8b
```

---

## Getting Help

### Before Reporting Issues

1. **Check logs:** `docker-compose logs -f voip-server`
2. **Test components:** Verify each service health
3. **Check config:** Validate `config.yaml`
4. **Try debug level:** Enable debug logging
5. **Reproduce:** Note exact steps to reproduce

### Information to Provide

- Server version
- Docker version (`docker --version`)
- `docker-compose logs` output
- `config.yaml` (sanitized)
- Steps to reproduce
- Expected vs actual behavior

### Support Channels

- **GitHub Issues:** https://github.com/your-org/voip-server/issues
- **Documentation:** See README.md and Wiki
- **Logs:** Include relevant log excerpts
