# Architecture Overview

## System Architecture

```
                    ┌─────────────────────────────────────┐
                    │           External World            │
                    │  (SIP Phones, WebRTC Clients,       │
                    │   Twilio/SignalWire Webhooks)       │
                    └──────────────┬────────────────┬───────┘
                                   │                │
                                   ▼                ▼
┌──────────────────────────────────────┐    ┌──────────────┐
│              SIP Handler             │    │   Webhooks   │
│         (github.com/emiago/sipgo)   │    │   (Twilio)   │
└──────────────┬───────────────────────┘    └──────┬───────┘
               │                                  │
               ▼                                  ▼
        ┌─────────────────┐              ┌──────────────┐
        │  Call Manager   │              │  WebSocket   │
        │  (State Machine)│              │  Server      │
        └────────┬────────┘              └──────┬───────┘
                 │                                │
                 │    ┌──────────────────────┐   │
                 │    │      Redis           │   │
                 │    │   (Session Store)    │   │
                 │    └──────────────────────┘   │
                 │                                │
                 └────────► ┌──────────┐ ◄──────┘
                            │  Audio   │
                            │ Pipeline │
                            └────┬─────┘
                                 │
            ┌────────────────────┼────────────────────┐
            │                    │                    │
            ▼                    ▼                    ▼
┌─────────────────┐    ┌─────────────────┐   ┌─────────────────┐
│   STT Engine    │    │   LLM Engine    │   │   TTS Engine    │
│  (faster-whisper│    │  (Ollama/vLLM)  │   │  (Piper/Coqui) │
│   + Vosk)       │    │                 │   │                 │
└─────────────────┘    └─────────────────┘   └─────────────────┘
            │                    │                    │
            │            ┌──────┴──────┐            │
            │            │  Transfer   │            │
            │            │   Engine    │            │
            │            └─────────────┘            │
            │                                       │
            └─────────────────┬─────────────────────┘
                              ▼
                     ┌─────────────────┐
                     │  Live Agents    │
                     │ (TUI + Web App) │
                     └─────────────────┘
```

## Call Flow

### New Call Lifecycle

```
INCOMING
    │
    ▼
┌─────────────┐     ┌──────────────┐     ┌──────────────┐
│ SIP INVITE  │────▶│ Create       │────▶│ LLM Routing  │
│ Received    │     │ Session      │     │              │
└─────────────┘     └──────────────┘     └──────┬───────┘
                                               │
                                               ▼
┌────────────────────────────────────────────────────────┐
│              LLM Routing Phase                          │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐          │
│  │ Capture  │──▶│  STT     │──▶│  LLM     │          │
│  │ Audio    │   │(Whisper) │   │(Ollama)  │          │
│  └──────────┘   └──────────┘   └────┬─────┘          │
│                                     │                  │
│                              ┌──────┴──────┐          │
│                              │  Transfer   │          │
│                              │  Engine     │          │
│                              │  Evaluate   │          │
│                              └──────┬──────┘          │
│                                     │                  │
│                              Transfer?                 │
│                           ┌────┴────┐                   │
│                          Yes       No                  │
│                           │          │                  │
│                           ▼          ▼                  │
│                    ┌──────────┐  ┌──────────┐          │
│                    │ TTS      │  │ LIVE     │          │
│                    │ Response │  │ AGENT    │          │
│                    │ (Piper)  │  │          │          │
│                    └──────────┘  └──────────┘          │
└────────────────────────────────────────────────────────┘
```

## Data Flow

### Audio Pipeline

```
Caller Audio (RTP/UDP)
       │
       ▼
┌──────────────┐
│ Audio Buffer │
│ (20ms frames)│
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Silence      │
│ Detection    │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Utterance    │
│ Complete     │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ STT Engine   │
│ (Whisper)    │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Transcript   │
│ Entry        │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ LLM Engine   │
│ (Ollama)     │
└──────┬───────┘
       │
       ▼
┌──────────────┐    ┌──────────────┐
│ Response     │───▶│ TTS Engine   │
│ JSON         │    │ (Piper)      │
└──────────────┘    └──────┬───────┘
                            │
                            ▼
                     Synthesized Audio
                            │
                            ▼
                     Caller Audio (RTP)
```

## Component Details

### 1. SIP Handler
- **Library**: `github.com/emiago/sipgo`
- **Protocols**: SIP v2.0, SDP, RTP
- **Codecs**: G.711 (PCMU/PCMA), Opus
- **Features**: INVITE, ACK, BYE, CANCEL, REGISTER, OPTIONS

### 2. Audio Pipeline
- **Buffer Size**: 320 samples (20ms @ 16kHz)
- **Silence Detection**: -40 dB threshold, 20 frame hangover
- **Threading**: Goroutine per call, non-blocking I/O

### 3. STT Engine
**Primary: faster-whisper**
- Model: base (can use tiny/small/medium/large-v3)
- Language: English (configurable)
- Device: CPU (GPU with CUDA optional)

**Fallback: Vosk**
- Port: 2700
- Model: en-US (offline)
- Latency: ~200ms

### 4. LLM Engine
**Primary: Ollama**
- Model: llama3.1:8b
- Temperature: 0.7
- Max Tokens: 512
- Response Format: JSON with intent, confidence, transfer flag

**Alternatives:**
- vLLM (high throughput)
- LocalAI (OpenAI-compatible API)

### 5. TTS Engine
**Primary: Piper**
- Voice: en_US-lessac-medium
- Speed: 1.0x
- Format: WAV (22050 Hz)

**Alternatives:**
- Coqui TTS (high quality, slower)
- Kokoro (Japanese-focused, optional)

### 6. Transfer Engine
- **Condition Rules**: Configurable via `config.yaml`
- **Triggers**: Intent match, confidence threshold, explicit request, max turns
- **Actions**: SIP REFER, provider bridge, agent queue

### 7. Session Store
- **Backend**: Redis
- **TTL**: 24 hours
- **Persistence**: JSON serialized
- **Pub/Sub**: Call events broadcast

## Performance Targets

| Metric | Target | Notes |
|--------|--------|-------|
| Concurrent Calls | 50+ | Depends on hardware |
| LLM Latency | < 2s | First byte response |
| STT Latency | < 500ms | Real-time streaming |
| TTS Latency | < 1s | First audio byte |
| Call Setup | < 500ms | SIP INVITE to 200 OK |
| Memory Usage | < 8GB | For 50 concurrent calls |

## Supported Integrations

### SIP Providers
- Twilio SIP
- SignalWire
- Vonage
- Local Asterisk/FreeSWITCH

### Webhook Providers
- **Twilio**: Voice XML, Stream API
- **SignalWire**: LaML, Relay API
- **Generic**: Custom HTTP POST with JSON

### Agent Interfaces
- **TUI (Terminal)**: Local agents, debugging
- **Web Dashboard**: Remote agents, supervisors
- **Mobile (future)**: Flutter app

## State Machine

```
                    ┌─────────────┐
     ┌─────────────▶│  INCOMING   │
     │              └──────┬──────┘
     │                     │
     │                     ▼
     │              ┌─────────────┐
     │              │ LLM_ROUTING │
     │              └──────┬──────┘
     │                     │
     │         ┌───────────┼───────────┐
     │         ▼           ▼           ▼
     │    ┌────────┐ ┌────────┐ ┌────────────┐
     │    │TRANSFER│ │ON_HOLD │ │TERMINATED  │
     │    │RING    │ │       │ │            │
     │    └───┬────┘ └───┬────┘ └────────────┘
     │        │          │           ▲
     │        ▼          │           │
     │   ┌──────────┐    │           │
     └───│LIVE_AGENT│─────┘───────────┘
         └──────────┘
```

**State Definitions:**

| State | Description | Allowed Transitions |
|-------|-------------|-------------------|
| INCOMING | Call just received | LLM_ROUTING, LIVE_AGENT, TRANSFER, TERMINATED |
| LLM_ROUTING | AI processing call | LIVE_AGENT, TERMINATED, TRANSFER, ON_HOLD |
| LIVE_AGENT | Human agent on call | ON_HOLD, TERMINATED, TRANSFER |
| TRANSFER | Transferring | LIVE_AGENT, TERMINATED |
| ON_HOLD | Placed on hold | LIVE_AGENT, LLM_ROUTING, TERMINATED |
| TERMINATED | Call ended | (none) |
