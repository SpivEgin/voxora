# Docker Deployment

## Overview

The VoIP server includes a complete Docker Compose setup for easy deployment. All self-hosted services (Redis, Ollama, Whisper, Piper) are containerized and pre-configured.

## Quick Start

```bash
# Clone and start
git clone https://github.com/your-org/voip-server.git
cd voip-server

# Start all services
docker-compose up -d

# Wait for services (30-60 seconds)
sleep 30

# Verify health
curl http://localhost:8080/health

# Pull LLM model (one-time)
curl -X POST http://localhost:11434/api/pull \
  -d '{"name": "llama3.1:8b"}'
```

## Services

| Service | Port | Purpose |
|---------|------|---------|
| voip-redis | 6379 | Hot cache / pub-sub |
| voip-ollama | 11434 | LLM inference engine |
| voip-whisper | 9090 | Speech-to-text |
| voip-piper | 5000 | Text-to-speech |
| voip-cockroachdb | 26257, 26258, 8081 | Distributed SQL database |
| voip-server | 8080, 5060 | Main VoIP server |

## Docker Compose Configuration

### Full Stack (`docker-compose.yml`)

```yaml
version: '3.8'

services:
  redis:
    image: redis:7-alpine
    container_name: voip-redis
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data
    restart: unless-stopped

  cockroachdb:
    image: cockroachdb/cockroach:latest
    container_name: voip-cockroachdb
    ports:
      - "26257:26257"
      - "26258:26258"
      - "8081:8081"
    volumes:
      - cockroach-data:/cockroach/cockroach-data
    environment:
      - COCKROACH_USER=root
      - COCKROACH_INSECURE=true
    command: start-single-node --insecure --store=type=mem,size=1GB --http-addr=0.0.0.0:8081 --sql-addr=0.0.0.0:26257 --listen-addr=0.0.0.0:26258
    restart: unless-stopped

  ollama:
    image: ollama/ollama:latest
    container_name: voip-ollama
    ports:
      - "11434:11434"
    volumes:
      - ollama-data:/root/.ollama
    environment:
      - OLLAMA_ORIGINS=*
    restart: unless-stopped

  whisper:
    image: fedirz/faster-whisper-server:latest-cpu
    container_name: voip-whisper
    ports:
      - "9090:8000"
    environment:
      - WHISPER_MODEL=base
      - WHISPER_LANGUAGE=en
    volumes:
      - whisper-cache:/root/.cache
    restart: unless-stopped

  piper:
    image: rhasspy/wyoming-piper:latest
    container_name: voip-piper
    ports:
      - "5000:10200"
    volumes:
      - piper-data:/data
    command: --voice en_US-lessac-medium
    restart: unless-stopped

  voip-server:
    build:
      context: ./voip-server
      dockerfile: Dockerfile
    container_name: voip-server
    ports:
      - "8080:8080"
      - "5060:5060/udp"
    depends_on:
      - redis
      - cockroachdb
      - ollama
      - whisper
      - piper
    volumes:
      - ./voip-server/configs:/app/configs:ro
    environment:
      - VOIP_REDIS_HOST=redis
      - VOIP_LLM_OLLAMA_HOST=ollama
      - VOIP_STT_WHISPER_HOST=whisper
      - VOIP_TTS_PIPER_HOST=piper
    restart: unless-stopped
```

### GPU-Accelerated Whisper

For NVIDIA GPUs, use the CUDA image:

```yaml
whisper:
  image: fedirz/faster-whisper-server:latest-cuda
  deploy:
    resources:
      reservations:
        devices:
          - driver: nvidia
            count: 1
            capabilities: [gpu]
```

### GPU-Accelerated Ollama

```yaml
ollama:
  image: ollama/ollama:latest
  deploy:
    resources:
      reservations:
        devices:
          - driver: nvidia
            count: 1
            capabilities: [gpu]
```

## Environment Variables

Set these in `docker-compose.yml` or `.env` file:

| Variable | Default | Description |
|----------|---------|-------------|
| `VOIP_REDIS_HOST` | redis | Redis hostname |
| `VOIP_REDIS_PASSWORD` | "" | Redis password |
| `VOIP_LLM_OLLAMA_HOST` | ollama | Ollama hostname |
| `VOIP_STT_WHISPER_HOST` | whisper | Whisper hostname |
| `VOIP_TTS_PIPER_HOST` | piper | Piper hostname |
| `VOIP_COCKROACHDB_HOST` | cockroachdb | CockroachDB hostname |
| `VOIP_COCKROACHDB_PORT` | 26257 | CockroachDB SQL port |
| `VOIP_COCKROACHDB_SSL_MODE` | disable | SSL mode for CockroachDB |
| `VOIP_LOG_LEVEL` | info | Log verbosity |

## Scaling

### Horizontal Scaling

For higher call volumes, scale the VoIP server:

```bash
# Start multiple instances
docker-compose up -d --scale voip-server=3

# With load balancer (nginx example)
upstream voip_backend {
    server voip-server_1:8080;
    server voip-server_2:8080;
    server voip-server_3:8080;
}
```

### GPU Clustering

For multiple GPUs, use Docker Swarm:

```bash
# Initialize swarm
docker swarm init

# Deploy stack
docker stack deploy -c docker-compose.yml voip

# Scale services
docker service scale voip_voip-server=5
```

## Persistent Storage

### Volumes

The following volumes are created:

| Volume | Service | Purpose |
|--------|---------|---------|
| redis-data | Redis | Session persistence |
| ollama-data | Ollama | Downloaded models (4GB+) |
| whisper-cache | Whisper | Model cache |
| piper-data | Piper | Voice data |
| cockroach-data | CockroachDB | Persistent transactional data |

### Backup

```bash
# Backup Redis
docker exec voip-redis redis-cli BGSAVE
docker cp voip-redis:/data/dump.rdb ./backup/

# Backup CockroachDB
docker exec voip-cockroachdb cockroach sql --insecure -e "BACKUP DATABASE * INTO 'nodelocal://1/backups'"
docker cp voip-cockroachdb:/cockroach/cockroach-data/backups ./backup/cockroachdb/

# Backup Ollama models
docker cp voip-ollama:/root/.ollama ./backup/
```

### Restore

```bash
# Restore Redis
docker cp ./backup/dump.rdb voip-redis:/data/
docker restart voip-redis

# Restore CockroachDB
docker cp ./backup/cockroachdb/ voip-cockroachdb:/cockroach/cockroach-data/restores/
docker exec voip-cockroachdb cockroach sql --insecure -e "RESTORE DATABASE * FROM 'nodelocal://1/restores' WITH OPTIONS (skip_missing_foreign_keys)"
```

## Network Configuration

### Custom Network

```yaml
networks:
  voip-network:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16
```

### External Access

For external SIP access, map port 5060:

```yaml
voip-server:
  ports:
    - "5060:5060/udp"
    - "10000-10100:10000-10100/udp"  # RTP range
```

## Health Checks

Services include built-in health checks:

```yaml
voip-server:
  healthcheck:
    test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
    interval: 30s
    timeout: 10s
    retries: 3
    start_period: 40s
```

## Resource Limits

### Production Limits

```yaml
voip-server:
  deploy:
    resources:
      limits:
        cpus: '2.0'
        memory: 4G
      reservations:
        cpus: '1.0'
        memory: 2G
```

## SSL/TLS

### Using nginx Reverse Proxy

```yaml
nginx:
  image: nginx:alpine
  ports:
    - "443:443"
  volumes:
    - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
    - ./certs:/etc/nginx/certs:ro
```

### nginx.conf

```nginx
server {
    listen 443 ssl;
    server_name voip.yourcompany.com;
    
    ssl_certificate /etc/nginx/certs/server.crt;
    ssl_certificate_key /etc/nginx/certs/server.key;
    
    location / {
        proxy_pass http://voip-server:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
    
    location /ws {
        proxy_pass http://voip-server:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

## Monitoring

### Prometheus Metrics

Enable metrics in config:

```yaml
metrics:
  enabled: true
  port: 9090
  path: "/metrics"
```

Add Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: 'voip-server'
    static_configs:
      - targets: ['voip-server:9090']
```

### Grafana Dashboard

Import dashboard ID `18828` (community VoIP dashboard) or create custom panels for:
- Active calls
- Call duration
- Transfer rate
- LLM latency
- STT accuracy

## Troubleshooting

### Container Won't Start

```bash
# Check logs
docker-compose logs -f voip-server

# Check resource usage
docker stats

# Verify ports
docker-compose ps
```

### Ollama Model Issues

```bash
# Re-pull model
docker exec voip-ollama ollama pull llama3.1:8b

# Check model list
docker exec voip-ollama ollama list
```

### Redis Connection Failures

```bash
# Test Redis
docker exec voip-redis redis-cli ping

# Check network
docker network inspect voip_default
```
