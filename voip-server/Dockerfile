# Multi-stage build for VoIP Server
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc g++ libc-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the server
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o voip-server cmd/server/main.go

# Build the TUI agent
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o tui-agent cmd/tui-agent/main.go

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates curl

# Create non-root user
RUN adduser -D -s /bin/sh voip

# Set working directory
WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/voip-server /app/
COPY --from=builder /app/tui-agent /app/

# Copy config
COPY configs/config.yaml /app/configs/

# Create log directory
RUN mkdir -p /var/log/voip-server && chown -R voip:voip /var/log/voip-server

# Set ownership
RUN chown -R voip:voip /app

# Switch to non-root user
USER voip

# Expose ports
EXPOSE 8080 5060/udp 5061/udp

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=30s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

# Default command
CMD ["./voip-server"]
