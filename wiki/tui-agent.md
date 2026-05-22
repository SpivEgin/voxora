# Using the TUI Agent

## Overview

The TUI (Terminal User Interface) Agent is a command-line tool for live agents to manage transferred calls. It provides real-time call information, transcript viewing, and call controls.

## Installation

```bash
# Build the TUI agent
make build-tui

# Or manually
cd voip-server
go build -o ../build/tui-agent ./cmd/tui-agent
```

## Starting the TUI

```bash
# With environment variables
export WS_URL="ws://localhost:8080/ws"
export AGENT_ID="agent_001"
./build/tui-agent

# Or let it generate an agent ID automatically
./build/tui-agent

# Override environment at runtime
WS_URL="ws://server:8080/ws" AGENT_ID="agent_002" ./build/tui-agent
```

## Interface Overview

```
┌─────────────────────────────────────────────────────────────────┐
│  🔊 VoIP Agent Dashboard — Agent: agent_001                      │
├───────────────────┬─────────────────────────────────────────────┤
│ Active Calls      │ Transcript                                  │
│                   │                                             │
│ [green]● caller_1   │ [cyan]caller:[-] Hello, I need help          │
│ State: LLM_ROUTING│                                            │
│ Intent: general   │ [green]llm:[-] Hello! How can I assist you     │
│ Dur: 00:45        │ today?                                      │
│                   │                                             │
│ [yellow]◐ caller_2   │ [yellow]agent:[-] I can help you with that.  │
│ State: TRANSFER   │                                             │
│ Intent: billing   │                                             │
│ Dur: 01:23        │                                             │
│                   │                                             │
│ [red]○ caller_3     │                                             │
│ State: TERMINATED │                                             │
│                   │                                             │
├───────────────────┴─────────────────────────────────────────────┤
│ Call Info          │ F1: Accept F2: Reject F3: Terminate F10: Help│
└─────────────────────────────────────────────────────────────────┘
```

## Navigation

### Panels

| Panel | Location | Content |
|-------|----------|---------|
| Call List | Left side | All active calls with status |
| Transcript | Top-right | Conversation history |
| Call Info | Bottom-right | Selected call details |
| Status Bar | Bottom | Agent ID, connection status, shortcuts |

### Moving Between Panels

- **Tab** or **Arrow Keys**: Navigate between panels
- **Enter**: Select a call from the list
- **Up/Down**: Scroll within panels

## Managing Calls

### Selecting a Call

1. Use **Tab** to focus the call list
2. Use **Up/Down arrows** to highlight a call
3. Press **Enter** to select

The transcript and info panels update automatically.

### Accepting a Transfer

When a transfer is requested:

1. A dialog appears showing:
   - Caller ID
   - Transfer reason (e.g., "low_confidence", "caller_requested_human")
2. Press **F1** or click **Accept**
3. The call state changes to `LIVE_AGENT`
4. You can now speak with the caller

### Rejecting a Transfer

1. When transfer dialog appears, press **F2** or click **Reject**
2. The call returns to LLM routing
3. You remain available for new transfers

### Terminating a Call

1. Select the call you want to end
2. Press **F3** or click **Terminate**
3. Confirm if prompted
4. The call state changes to `TERMINATED`

## Keyboard Shortcuts

| Key | Action | Context |
|-----|--------|---------|
| **F1** | Accept transfer | Transfer dialog |
| **F2** | Reject transfer | Transfer dialog |
| **F3** | Terminate call | Any time |
| **F10** | Show help | Any time |
| **ESC** | Quit application | Any time |
| **Tab** | Next panel | Navigation |
| **↑ / ↓** | Scroll/Select | Call list |
| **Enter** | Select call | Call list |
| **Page Up** | Scroll up | Transcript |
| **Page Down** | Scroll down | Transcript |

## Call Status Icons

| Icon | Color | Meaning |
|------|-------|---------|
| ● | Green | Active call |
| ◐ | Yellow | Transferring/Waiting |
| ○ | Red | Terminated |
| 📞 | White | Incoming |
| 🤖 | White | LLM routing |
| 👤 | White | Live agent |

## Transcript Colors

| Speaker | Color | Example |
|---------|-------|---------|
| Caller | Cyan | `[cyan]caller:[-]` |
| LLM | Green | `[green]llm:[-]` |
| Agent | Yellow | `[yellow]agent:[-]` |

## Call Information Panel

Displays:
- **Call ID**: Unique session identifier
- **Caller**: Phone number or ID
- **State**: Current call state
- **Intent**: Detected intent
- **Confidence**: LLM confidence score (0.0 - 1.0)
- **Duration**: Elapsed time (MM:SS)

## Troubleshooting

### Connection Issues

**Disconnected status shown:**
```
1. Check server is running: curl http://localhost:8080/health
2. Verify WebSocket URL: WS_URL="ws://correct-host:8080/ws"
3. Check firewall: Port 8080 must be open
4. Restart TUI: Press ESC and restart
```

**No calls appearing:**
```
1. Verify agent is logged in (status shows "Connected")
2. Check SIP server is receiving calls
3. Ensure transfer conditions are configured
4. Check server logs for errors
```

### Display Issues

**Garbled text:**
```
1. Ensure terminal supports Unicode: locale should be UTF-8
2. Use a modern terminal (iTerm2, Windows Terminal, Alacritty)
3. Set TERM=xterm-256color
```

**Colors not showing:**
```
1. Enable 256-color mode: export TERM=xterm-256color
2. Use a terminal with true color support
3. Check COLORTERM=truecolor
```

## Advanced Usage

### Multiple Terminals

Run multiple TUI agents simultaneously:

```bash
# Terminal 1 - Agent for Sales
export AGENT_ID="sales_agent_001"
export WS_URL="ws://server:8080/ws"
./build/tui-agent

# Terminal 2 - Agent for Support
export AGENT_ID="support_agent_002"
export WS_URL="ws://server:8080/ws"
./build/tui-agent
```

### Logging

The TUI logs to stdout. Redirect to a file:

```bash
./build/tui-agent 2>&1 | tee tui-agent.log
```

### Custom Configuration

Create a wrapper script:

```bash
#!/bin/bash
# tui-wrapper.sh
export WS_URL="ws://prod-server:8080/ws"
export AGENT_ID="agent_${USER}"
export TERM="xterm-256color"
./build/tui-agent "$@"
```

## Best Practices

1. **Always accept transfers promptly** - Callers may be waiting
2. **Review transcript before accepting** - Understand the context
3. **Use F10 for help** - Quick reference of shortcuts
4. **Scroll transcript** - Use Page Up/Down for history
5. **Monitor connection status** - Green = connected, Red = disconnected

## Keyboard Shortcut Cheat Sheet

```
F1  → Accept transfer
F2  → Reject transfer
F3  → Terminate call
F10 → Show this help
ESC → Quit application
↑/↓ → Navigate call list
Tab → Switch panels
PgUp/PgDn → Scroll transcript
```
