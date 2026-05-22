# Security

## Overview

This guide covers security best practices for deploying the VoIP server in production environments.

## TLS/SRTP

### Enabling TLS for SIP

```yaml
server:
  sip:
    tls_enabled: true
    tls_port: 5061
    auth:
      cert_file: "/etc/voip-server/certs/server.crt"
      key_file: "/etc/voip-server/certs/server.key"
```

**Certificate Requirements:**
- X.509 v3 certificate
- RSA 2048-bit or ECDSA P-256 key
- Valid hostname matching SIP domain
- Not expired

**Generating Self-Signed Certificates (Development):**

```bash
openssl req -x509 -newkey rsa:2048 \
  -keyout server.key -out server.crt \
  -days 365 -nodes \
  -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
```

### SRTP for Media

```yaml
security:
  srtp:
    enabled: true
    crypto_suites:
      - "AES_CM_128_HMAC_SHA1_80"
      - "AES_CM_128_HMAC_SHA1_32"
```

**Crypto Suites:**

| Suite | Encryption | Authentication | Key Length |
|-------|-----------|----------------|------------|
| AES_CM_128_HMAC_SHA1_80 | AES-128 | HMAC-SHA1 | 80-bit tag |
| AES_CM_128_HMAC_SHA1_32 | AES-128 | HMAC-SHA1 | 32-bit tag |

**Enabling SRTP in SDP:**

```
m=audio 10000 RTP/SAVP 0 8 96
a=crypto:1 AES_CM_128_HMAC_SHA1_80 inline:...
```

## Authentication

### SIP Digest Authentication

```yaml
server:
  sip:
    auth:
      enabled: true
      realm: "voip.yourcompany.com"
```

**Realm Configuration:**
- Use a domain you control
- Match TLS certificate CN/SAN
- Include in SIP URI: `sip:user@voip.yourcompany.com`

### Agent Authentication

```yaml
agents:
  auth:
    enabled: true
    jwt_secret: "your-256-bit-secret-key-here"
    token_expiry: 3600  # 1 hour
```

**JWT Token Format:**

```json
{
  "sub": "agent_001",
  "name": "John Doe",
  "role": "agent",
  "iat": 1653240000,
  "exp": 1653243600
}
```

**Generating Secure Secret:**

```bash
openssl rand -base64 32
# or
dd if=/dev/urandom bs=1 count=32 | base64
```

### API Authentication

Add Bearer token to requests:

```bash
curl -H "Authorization: Bearer <-jwt-token>" \
  http://localhost:8080/api/v1/calls
```

## Rate Limiting

```yaml
security:
  rate_limit:
    enabled: true
    requests_per_second: 100
    burst: 50
```

**Default Limits:**

| Endpoint | Limit | Burst |
|----------|-------|-------|
| `/health` | Unlimited | N/A |
| `/api/v1/*` | 100/sec | 50 |
| `/ws` | 10 connections/min | N/A |
| Webhooks | 50/sec | 25 |

**Custom Per-IP Limits:**

```yaml
security:
  rate_limit:
    enabled: true
    requests_per_second: 100
    burst: 50
    per_ip: true
```

## CORS Configuration

### Development

```yaml
# config.yaml - Allow all origins (development only!)
server:
  rest:
    cors:
      enabled: true
      allowed_origins: ["*"]
      allowed_methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
      allowed_headers: ["*"]
      allow_credentials: false
```

### Production

```yaml
# config.yaml - Restricted origins
server:
  rest:
    cors:
      enabled: true
      allowed_origins:
        - "https://dashboard.yourcompany.com"
        - "https://agents.yourcompany.com"
      allowed_methods: ["GET", "POST"]
      allowed_headers:
        - "Authorization"
        - "Content-Type"
      allow_credentials: true
```

## Network Security

### Firewall Rules

**Recommended iptables rules:**

```bash
# SIP UDP
tables -A INPUT -p udp --dport 5060 -j ACCEPT

# SIP TLS
tables -A INPUT -p tcp --dport 5061 -j ACCEPT

# HTTP API/WebSocket
iptables -A INPUT -p tcp --dport 8080 -s 10.0.0.0/24 -j ACCEPT

# Redis (internal only)
iptables -A INPUT -p tcp --dport 6379 -s 172.20.0.0/16 -j DROP

# Default deny
iptables -P INPUT DROP
```

### Docker Network Isolation

```yaml
# docker-compose.yml
services:
  redis:
    networks:
      - backend
  voip-server:
    networks:
      - backend
      - frontend
    ports:
      - "8080:8080"
      - "5060:5060/udp"

networks:
  backend:
    internal: true  # No external access
  frontend:
    driver: bridge
```

## Data Encryption

### Redis Encryption

Use Redis with TLS:

```bash
# redis.conf
port 0
tls-port 6379
tls-cert-file /path/to/redis.crt
tls-key-file /path/to/redis.key
tls-ca-cert-file /path/to/ca.crt
```

### Configuration Encryption

**Never commit credentials to git:**

```yaml
# config.yaml (safe to commit)
redis:
  password: "${REDIS_PASSWORD}"  # From env var

# .env (never commit, use gitignore)
REDIS_PASSWORD=super-secret-password
```

## Audit Logging

Enable audit events:

```yaml
logging:
  level: "info"
  audit:
    enabled: true
    events:
      - call_start
      - call_end
      - transfer
      - agent_login
      - agent_logout
      - config_change
```

**Audit Log Format:**

```json
{
  "timestamp": "2026-05-22T12:00:00Z",
  "event": "call_start",
  "session_id": "abc-123-def",
  "caller_id": "+1234567890",
  "agent_id": null,
  "ip_address": "192.168.1.100",
  "details": {}
}
```

## Compliance

### GDPR Considerations

- **Data Retention**: Configure automatic cleanup:
  ```yaml
  logging:
    retention_days: 30
  ```
- **Right to Erasure**: API endpoint for deletion:
  ```bash
  curl -X DELETE http://localhost:8080/api/v1/calls/:id/recordings
  ```
- **Consent Logging**: Track consent status per call

### PCI DSS (if handling payments)

- Never log full card numbers
- Use tokenization for payment data
- Enable audit logging for all access
- Regular security scans

## Security Checklist

- [ ] TLS enabled for SIP signaling
- [ ] SRTP enabled for media
- [ ] JWT authentication enabled
- [ ] Strong JWT secret (256-bit)
- [ ] Rate limiting configured
- [ ] CORS restricted in production
- [ ] Redis password set
- [ ] Internal services on private networks
- [ ] Firewall rules configured
- [ ] Audit logging enabled
- [ ] Log retention configured
- [ ] Security headers set
- [ ] Regular updates scheduled
