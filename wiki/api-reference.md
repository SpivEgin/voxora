# API Reference

## REST API

### Base URL

```
http://localhost:8080/api/v1
```

### Authentication

API endpoints support Bearer token authentication when enabled in config:

```bash
Authorization: Bearer <your-jwt-token>
```

---

## Call Management

### List All Calls

```http
GET /calls?state=ACTIVE
```

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| state | string | No | Filter by state: `INCOMING`, `LLM_ROUTING`, `LIVE_AGENT`, `TRANSFERRING`, `TERMINATED` |

**Response (200 OK):**

```json
{
  "calls": [
    {
      "id": "abc-123-def",
      "state": "LLM_ROUTING",
      "caller_id": "+1234567890",
      "caller_number": "+1234567890",
      "called_number": "+0987654321",
      "duration": 45,
      "intent": "general",
      "confidence": 0.87,
      "agent_id": null,
      "agent_name": null
    }
  ],
  "count": 1
}
```

### Get Call Details

```http
GET /calls/:id
```

**Response (200 OK):**

```json
{
  "id": "abc-123-def",
  "state": "LIVE_AGENT",
  "caller_id": "+1234567890",
  "caller_number": "+1234567890",
  "called_number": "+0987654321",
  "start_time": "2026-05-22T12:00:00Z",
  "duration": 120,
  "llm_turn_count": 5,
  "transcript": [
    {
      "timestamp": "2026-05-22T12:00:05Z",
      "speaker": "caller",
      "text": "Hello, I need help",
      "audio_url": ""
    }
  ],
  "llm_summary": "Customer needs technical support",
  "transfer_reason": "low_confidence",
  "intent": "technical_support",
  "confidence": 0.45,
  "agent_id": "agent_001",
  "agent_name": "John Doe"
}
```

### Terminate Call

```http
POST /calls/:id/terminate
```

**Response (200 OK):**

```json
{
  "status": "terminated",
  "id": "abc-123-def"
}
```

### Transfer Call

```http
POST /calls/:id/transfer
```

**Request Body:**

```json
{
  "target": "support",
  "reason": "technical_issue"
}
```

**Response (200 OK):**

```json
{
  "status": "transfer_requested",
  "id": "abc-123-def",
  "target": "support",
  "sip_uri": "sip:support@queue.local"
}
```

### Get Call Transcript

```http
GET /calls/:id/transcript
```

**Response (200 OK):**

```json
{
  "id": "abc-123-def",
  "transcript": [
    {
      "timestamp": "2026-05-22T12:00:05Z",
      "speaker": "caller",
      "text": "Hello, I need help",
      "audio_url": ""
    },
    {
      "timestamp": "2026-05-22T12:00:08Z",
      "speaker": "llm",
      "text": "Hello! How can I assist you today?",
      "audio_url": ""
    }
  ]
}
```

---

## Agent Management

### List Agents

```http
GET /agents
```

**Response (200 OK):**

```json
{
  "agents": [
    {
      "id": "agent_001",
      "name": "John Doe",
      "status": "available",
      "calls": 0
    },
    {
      "id": "agent_002",
      "name": "Jane Smith",
      "status": "busy",
      "calls": 1
    }
  ],
  "count": 2
}
```

### Get Agent Calls

```http
GET /agents/:id/calls
```

**Response (200 OK):**

```json
{
  "agent_id": "agent_001",
  "calls": [
    {
      "id": "abc-123-def",
      "state": "LIVE_AGENT",
      "caller_id": "+1234567890",
      "duration": 120,
      "intent": "technical_support"
    }
  ],
  "count": 1
}
```

### Agent Login

```http
POST /agents/:id/login
```

**Response (200 OK):**

```json
{
  "status": "logged_in",
  "agent_id": "agent_001"
}
```

### Agent Logout

```http
POST /agents/:id/logout
```

**Response (200 OK):**

```json
{
  "status": "logged_out",
  "agent_id": "agent_001"
}
```

---

## System

### Get System Stats

```http
GET /system/stats
```

**Response (200 OK):**

```json
{
  "stats": {
    "active_calls": 3,
    "audio_sessions": 3,
    "sip_calls": 2,
    "timestamp": 1653240000
  }
}
```

### Get Health Status

```http
GET /system/health
```

**Response (200 OK):**

```json
{
  "status": {
    "sip": "healthy",
    "redis": "healthy",
    "stt": "healthy",
    "tts": "healthy",
    "llm": "healthy"
  },
  "time": 1653240000
}
```

---

## Transfer Targets

### List Transfer Targets

```http
GET /transfer/targets
```

**Response (200 OK):**

```json
{
  "targets": [
    {
      "name": "sales",
      "sip_uri": "sip:sales@queue.local",
      "priority": 1
    },
    {
      "name": "support",
      "sip_uri": "sip:support@queue.local",
      "priority": 2
    },
    {
      "name": "billing",
      "sip_uri": "sip:billing@queue.local",
      "priority": 3
    }
  ]
}
```

---

## WebSocket Events

### Connection

**Endpoint:**
```
ws://localhost:8080/ws
```

**Headers:**
```
Origin: http://localhost:8081
```

### Client to Server Messages

#### agent_login

```json
{
  "type": "agent_login",
  "payload": {
    "agent_id": "agent_001"
  }
}
```

#### agent_logout

```json
{
  "type": "agent_logout",
  "payload": {}
}
```

#### subscribe_calls

```json
{
  "type": "subscribe_calls",
  "payload": {}
}
```

#### join_call

```json
{
  "type": "join_call",
  "payload": {
    "session_id": "abc-123-def"
  }
}
```

#### leave_call

```json
{
  "type": "leave_call",
  "payload": {
    "session_id": "abc-123-def"
  }
}
```

#### accept_transfer

```json
{
  "type": "accept_transfer",
  "payload": {
    "session_id": "abc-123-def"
  }
}
```

#### reject_transfer

```json
{
  "type": "reject_transfer",
  "payload": {
    "session_id": "abc-123-def"
  }
}
```

#### audio_data

```json
{
  "type": "audio_data",
  "payload": {
    "session_id": "abc-123-def",
    "data": "base64_encoded_audio_data"
  }
}
```

### Server to Client Messages

#### active_calls

```json
{
  "type": "active_calls",
  "payload": {
    "calls": [
      {
        "id": "abc-123-def",
        "state": "LLM_ROUTING",
        "caller_id": "+1234567890",
        "duration": 45,
        "intent": "general",
        "confidence": 0.87
      }
    ]
  }
}
```

#### call_started

```json
{
  "type": "call_started",
  "payload": {
    "id": "abc-123-def",
    "state": "INCOMING",
    "caller_id": "+1234567890",
    "duration": 0,
    "intent": "",
    "confidence": 0.0
  }
}
```

#### call_ended

```json
{
  "type": "call_ended",
  "payload": {
    "session_id": "abc-123-def"
  }
}
```

#### transcript_update

```json
{
  "type": "transcript_update",
  "payload": {
    "session_id": "abc-123-def",
    "speaker": "caller",
    "text": "Hello, I need help"
  }
}
```

#### transfer_requested

```json
{
  "type": "transfer_requested",
  "payload": {
    "session_id": "abc-123-def",
    "caller_id": "+1234567890",
    "reason": "low_confidence",
    "intent": "billing_inquiry"
  }
}
```

#### state_change

```json
{
  "type": "state_change",
  "payload": {
    "session_id": "abc-123-def",
    "old_state": "LLM_ROUTING",
    "new_state": "LIVE_AGENT"
  }
}
```

#### error

```json
{
  "type": "error",
  "payload": {
    "message": "Invalid session ID"
  }
}
```

#### audio_data

```json
{
  "type": "audio_data",
  "payload": {
    "session_id": "abc-123-def",
    "data": "base64_encoded_audio_data"
  }
}
```

---

## Webhooks

### Twilio

**Endpoint:**
```
POST /webhooks/twilio
```

**Expected Parameters:**
- `CallSid`: Twilio call SID
- `From`: Caller number
- `To`: Called number
- `CallStatus`: Call status (ringing, in-progress, completed)

**Response:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Connect>
    <Stream url="ws://your-server:8080/ws" />
  </Connect>
</Response>
```

### SignalWire

**Endpoint:**
```
POST /webhooks/signalwire
```

**Expected Parameters:**
- `call_sid`: SignalWire call SID
- `from_number`: Caller number
- `to_number`: Called number
- `call_status`: Call status

**Response:**
```json
{
  "status": "received"
}
```

---

## Response Codes

| Code | Meaning | Usage |
|------|---------|-------|
| 200 | OK | Successful GET, POST |
| 201 | Created | Reserved for future use |
| 400 | Bad Request | Invalid parameters |
| 401 | Unauthorized | Missing/invalid auth |
| 403 | Forbidden | Insufficient permissions |
| 404 | Not Found | Call/agent not found |
| 409 | Conflict | Call state invalid |
| 500 | Internal Error | Server error |
| 503 | Service Unavailable | System not ready |

## Error Format

```json
{
  "error": "Human-readable error message",
  "code": "ERROR_CODE",
  "details": {}
}
```

## Rate Limiting

When enabled in config:

- **Limit**: 100 requests/second
- **Burst**: 50 requests
- **Headers**:
  - `X-RateLimit-Limit`: 100
  - `X-RateLimit-Remaining`: 95
  - `X-RateLimit-Reset`: 1653240000

## Example cURL Commands

```bash
# Health check
curl http://localhost:8080/health

# List calls
curl http://localhost:8080/api/v1/calls | jq

# Get specific call
curl http://localhost:8080/api/v1/calls/abc-123-def

# Terminate call
curl -X POST http://localhost:8080/api/v1/calls/abc-123-def/terminate

# Transfer call
curl -X POST http://localhost:8080/api/v1/calls/abc-123-def/transfer \
  -d '{"target": "support", "reason": "technical_issue"}'

# Get transcript
curl http://localhost:8080/api/v1/calls/abc-123-def/transcript

# System stats
curl http://localhost:8080/api/v1/system/stats

# Health status
curl http://localhost:8080/api/v1/system/health
```
