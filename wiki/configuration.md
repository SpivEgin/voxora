# Configuration Guide

## Overview

The VoIP server uses **YAML** configuration files with **environment variable** overrides. The configuration is loaded from the first available path in this order:

1. `./configs/config.yaml` (development)
2. `/etc/voip-server/config.yaml` (system-wide)
3. `~/.config/voip-server/config.yaml` (user-specific)

## Quick Reference

| Setting | Default | Description |
|---------|---------|-------------|
| `server.sip.port` | 5060 | SIP UDP port |
| `server.rest.port` | 8080 | HTTP API/WebSocket port |
| `redis.host` | localhost | Redis server address |
| `stt.engine` | whisper | STT engine: `whisper` or `vosk` |
| `llm.backend` | ollama | LLM backend: `ollama`, `vllm`, or `localai` |
| `tts.engine` | piper | TTS engine: `piper`, `coqui`, or `kokoro` |

## Server Settings

### SIP Configuration

```yaml
server:
  sip:
    enabled: true
    host: "0.0.0.0"
    port: 5060
    tls_port: 5061
    tls_enabled: false
    domain: "localhost"
    user_agent: "VoIP-Server/1.0"
    auth:
      enabled: false
      realm: "voip.local"
      cert_file: ""      # Required if tls_enabled
      key_file: ""       # Required if tls_enabled
```

**Key Options:**
- `enabled`: Enable/disable SIP server
- `host`: Bind address (use `"0.0.0.0"` for all interfaces)
- `port`: SIP UDP port (standard: 5060)
- `tls_enabled`: Enable SIP over TLS (requires cert/key files)
- `auth.enabled`: Enable digest authentication

## CockroachDB Configuration

```yaml
cockroachdb:
  host: "localhost"
  port: 26257
  database: "voipdb"
  user: "root"
  password: ""
  ssl_mode: "disable"
  ssl_root_cert: ""
  ssl_cert: ""
  ssl_key: ""
  pool_size: 10
  max_retries: 3
  retry_delay: 1s
  migrations:
    enabled: true
    path: "./migrations"
```

**SSL Mode Options:**
- `disable`: No SSL/TLS encryption (development only)
- `require`: SSL/TLS required, but skip certificate verification
- `verify-ca`: Verify server certificate against CA
- `verify-full`: Verify certificate and hostname (most secure, recommended for production)

**Migration Settings:**
- `enabled`: Automatically run database migrations on startup
- `path`: Directory containing `.sql` migration files

### REST API & WebSocket

```yaml
server:
  rest:
    enabled: true
    host: "0.0.0.0"
    port: 8080
  websocket:
    enabled: true
    host: "0.0.0.0"
    port: 8080
    path: "/ws"
```

**Note:** REST and WebSocket can share the same port.

## Redis Session Store

```yaml
redis:
  host: "localhost"
  port: 6379
  password: ""
  database: 0
  pool_size: 10
  min_idle_conns: 5
```

**Security:**
- Use a strong Redis password in production
- Enable Redis persistence for call history
- Consider Redis Sentinel for high availability

## Speech-to-Text (STT)

### Primary: faster-whisper

```yaml
stt:
  engine: "whisper"
  whisper:
    host: "localhost"
    port: 9090
    model: "base"      # Options: tiny, base, small, medium, large-v3
    language: "en"
    device: "cpu"      # Use "cuda" for GPU
    compute_type: "int8"  # Options: int8, float16, float32
```

**Model Selection:**

| Model | Size | Accuracy | Speed | VRAM Required |
|-------|------|----------|-------|---------------|
| tiny | 39MB | Low | Fastest | ~1GB |
| base | 74MB | Good | Very Fast | ~1GB |
| small | 244MB | Better | Fast | ~2GB |
| medium | 769MB | High | Medium | ~5GB |
| large-v3 | 1550MB | Highest | Slow | ~10GB |

### Fallback: Vosk

```yaml
stt:
  engine: "vosk"
  vosk:
    host: "localhost"
    port: 2700
    model_path: "/opt/vosk-model"
    sample_rate: 16000
```

## LLM Configuration

### Primary: Ollama

```yaml
llm:
  backend: "ollama"
  ollama:
    host: "localhost"
    port: 11434
    model: "llama3.1:8b"
    temperature: 0.7
    max_tokens: 512
    timeout: 30
```

**Recommended Models:**

| Model | Size | Speed | Quality | Use Case |
|-------|------|-------|---------|----------|
| llama3.1:8b | 8B | Fast | Good | General purpose |
| mistral:7b | 7B | Fast | Good | Balanced |
| phi3:mini | 3.8B | Very Fast | Moderate | Low-resource |
| llama3.1:70b | 70B | Slow | Excellent | High quality |

### System Prompt

The system prompt controls AI behavior:

```yaml
llm:
  ollama:
    system_prompt: |
      You are a helpful voice assistant for a customer service system.
      Your job is to understand the caller's intent and help them or
      route to a human agent when appropriate.
      
      Respond in a natural, conversational way. Keep responses concise.
      
      IMPORTANT: Respond with valid JSON in this format:
      {
        "response": "Your spoken response",
        "transfer_to_agent": false,
        "reason": null,
        "confidence": 0.85,
        "intent": "billing_inquiry|technical_support|sales|complaint|refund|emergency|general"
      }
      
      Set transfer_to_agent to true if:
      - The caller explicitly asks for a human agent
      - You detect an emergency situation
      - The issue requires human judgment
      - Your confidence is below 0.6
```

**Customizing the Prompt:**
- Adjust transfer conditions
- Add company-specific instructions
- Change response style (formal, casual, etc.)
- Add language-specific rules

## Text-to-Speech (TTS)

### Primary: Piper

```yaml
tts:
  engine: "piper"
  piper:
    host: "localhost"
    port: 5000
    voice: "en_US-lessac-medium"
    speed: 1.0
```

**Available Voices:**
- `en_US-lessac-medium` (default, good quality)
- `en_US-lessac-high` (higher quality, slower)
- `en_GB-northern_english_male-medium`
- `en_US-amy-medium` (female voice)

## Transfer Rules

### Transfer Conditions

```yaml
transfer:
  enabled: true
  conditions:
    # Caller explicitly requests human
    caller_requests_human: true
    
    # LLM confidence threshold (0.0 - 1.0)
    confidence_threshold: 0.6
    
    # Specific intents trigger immediate transfer
    intent_triggers:
      - "complaint"
      - "refund"
      - "emergency"
      - "billing_dispute"
      - "legal"
      - "supervisor"
    
    # Maximum LLM interactions before transfer
    max_llm_turns: 10
    
    # Maximum call duration in seconds
    max_call_duration: 600
    
    # Enable profanity/abuse detection
    abuse_detection: true
    
    # Silence timeout in seconds
    silence_timeout: 30
```

### Transfer Targets

```yaml
transfer:
  targets:
    - name: "sales"
      sip_uri: "sip:sales@queue.local"
      priority: 1
      
    - name: "support"
      sip_uri: "sip:support@queue.local"
      priority: 2
      
    - name: "billing"
      sip_uri: "sip:billing@queue.local"
      priority: 3
      
    - name: "emergency"
      sip_uri: "sip:emergency@queue.local"
      priority: 0
      available_24h: true
```

**Priority:** Lower number = higher priority. `0` = highest priority.

## Audio Pipeline

```yaml
audio:
  sample_rate: 16000
  frame_duration: 20
  codecs:
    - "opus"
    - "pcmu"
    - "pcma"
  buffer_size: 320
  silence_detection:
    enabled: true
    threshold: -40    # dB
    hangover: 20    # frames
```

**Silence Detection:**
- **threshold**: Audio below this level is considered silence (-40 dB)
- **hangover**: Number of silent frames before ending utterance (20 frames = 400ms)

## Logging

```yaml
logging:
  level: "info"      # Options: debug, info, warn, error
  format: "json"     # Options: json, console
  output: "stdout"   # Options: stdout, file path
```

**Log Levels:**
- `debug`: Detailed information, useful for development
- `info`: General operational information (default)
- `warn`: Warning messages, non-critical issues
- `error`: Error messages, requires attention

## Security

```yaml
security:
  tls:
    enabled: false
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"
    
  srtp:
    enabled: true
    crypto_suites:
      - "AES_CM_128_HMAC_SHA1_80"
      - "AES_CM_128_HMAC_SHA1_32"
      
  rate_limit:
    enabled: true
    requests_per_second: 100
    burst: 50
```

## Environment Variables

All configuration values can be overridden using environment variables:

```bash
# Redis
VOIP_REDIS_HOST=10.0.0.1
VOIP_REDIS_PASSWORD=secret

# LLM
VOIP_LLM_OLLAMA_HOST=192.168.1.50
VOIP_LLM_OLLAMA_MODEL="mistral:7b"

# CockroachDB
VOIP_COCKROACHDB_HOST=localhost
VOIP_COCKROACHDB_PORT=26257
VOIP_COCKROACHDB_DATABASE=voipdb
VOIP_COCKROACHDB_USER=root
VOIP_COCKROACHDB_PASSWORD=secret
VOIP_COCKROACHDB_SSL_MODE=verify-full
VOIP_COCKROACHDB_SSL_ROOT_CERT="/certs/ca.crt"
VOIP_COCKROACHDB_SSL_CERT="/certs/client.crt"
VOIP_COCKROACHDB_SSL_KEY="/certs/client.key"
VOIP_COCKROACHDB_POOL_SIZE=10
VOIP_COCKROACHDB_MIGRATIONS_ENABLED=true
VOIP_COCKROACHDB_MIGRATIONS_PATH="./migrations"

# STT
VOIP_STT_WHISPER_DEVICE="cuda"
VOIP_STT_WHISPER_MODEL="small"

# Transfer
VOIP_TRANSFER_ENABLED=true
VOIP_TRANSFER_CONDITIONS_CONFIDENCE_THRESHOLD=0.7
```

**Naming Convention:**
- Replace dots with underscores
- Prefix with `VOIP_`
- Example: `server.sip.port` → `VOIP_SERVER_SIP_PORT`

## Production Configuration Example

```yaml
server:
  environment: "production"
  sip:
    enabled: true
    host: "0.0.0.0"
    port: 5060
    tls_enabled: true
    auth:
      enabled: true
  rest:
    enabled: true
    host: "0.0.0.0"
    port: 8080

redis:
  host: "redis.voip.internal"
  password: "${REDIS_PASSWORD}"

stt:
  engine: "whisper"
  whisper:
    host: "whisper.voip.internal"
    model: "small"
    device: "cuda"

llm:
  backend: "ollama"
  ollama:
    host: "ollama.voip.internal"
    model: "llama3.1:8b"

tts:
  engine: "piper"
  piper:
    host: "piper.voip.internal"

transfer:
  enabled: true
  conditions:
    confidence_threshold: 0.6
    max_llm_turns: 5
    max_call_duration: 300

logging:
  level: "warn"
  format: "json"
  output: "/var/log/voip-server/app.log"

security:
  tls:
    enabled: true
    cert_file: "/certs/server.crt"
    key_file: "/certs/server.key"
  rate_limit:
    enabled: true
    requests_per_second: 50
```
