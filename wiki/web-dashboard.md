# Using the Web Dashboard

## Overview

The Web Dashboard is a browser-based interface for live agents. It provides the same functionality as the TUI Agent but through a web interface accessible from any modern browser.

## Accessing the Dashboard

### Building

```bash
# Compile Dart to JavaScript
make build-web

# Or manually
cd agent-web
dart compile js -O2 -o web/main.dart.js web/main.dart
```

### Serving

```bash
# Using make
make run-web

# Or manually (Python)
cd agent-web/web
python3 -m http.server 8081

# Or using Node.js
cd agent-web/web
npx http-server -p 8081

# Or using nginx
# Place files in /var/www/html
```

### Accessing

Open a web browser and navigate to:

```
http://localhost:8081
```

If the server is running on a different machine:

```
http://your-server-ip:8081
```

## Dashboard Layout

```
┌─────────────────────────────────────────────────────────────────┐
│  🔊 VoIP Agent Dashboard                    Agent: agent_001     │
│                                        [Connected] [Refresh] [Logout] │
├───────────────────┬─────────────────────────────────────────────┤
│                   │                                             │
│ Active Calls      │ Transcript                                  │
│ ═══════════════   │ ═══════════════════════════════════════════│
│                   │                                             │
│ ⬤ caller_1      │ [09:15] caller: Hello, I need help         │
│ INCOMING          │                                            │
│ general | 87%     │ [09:16] llm: Hello! How can I assist      │
│ 00:45             │         you today?                          │
│                   │                                            │
│ ⬤ caller_2      │ [09:17] agent: I can help you with that.   │
│ LLM_ROUTING       │                                            │
│ billing | 45%     │                                            │
│ 01:23             │                                            │
│                   │                                            │
│ ⬤ caller_3      │                                            │
│ TRANSFERRING      │                                            │
│ complaint | 92%   │                                            │
│ 00:30             │                                            │
│                   │                                            │
├───────────────────┴─────────────────────────────────────────────┤
│ Call Information                                               │
│ ════════════════════════════════════════════════════════════════ │
│ Call ID:   abc-123-def                                          │
│ Caller:    +1234567890                                          │
│ State:     LLM_ROUTING                                          │
│ Intent:    general                                              │
│ Confidence: 87.0%                                               │
│ Duration:  00:45                                                │
├─────────────────────────────────────────────────────────────────┤
│ [✓ Accept (F1)]  [✕ Reject (F2)]  [⏹ Terminate (F3)]        │
└─────────────────────────────────────────────────────────────────┘
```

## Features

### Real-Time Call List

The left panel shows all active calls with:

| Column | Description |
|--------|-------------|
| Status Icon | ● = Active, ◐ = Transferring, ○ = Terminated |
| Caller ID | Phone number or identifier |
| State | Current call state (INCOMING, LLM_ROUTING, etc.) |
| Intent | Detected caller intent |
| Confidence | LLM confidence percentage |
| Duration | Elapsed time |

### Live Transcript

The top-right panel displays the conversation with color-coded speakers:

| Speaker | Color | Example |
|---------|-------|---------|
| Caller | Blue highlight | "Hello, I need help" |
| LLM | Yellow highlight | "How can I assist you?" |
| Agent | Purple highlight | "I can help with that." |

**Features:**
- Auto-scrolls to latest message
- Click transcript to scroll through history
- Timestamps on each message
- Speaker identification

### Call Information

The bottom-right panel shows detailed call info:
- **Call ID**: Unique session identifier
- **Caller**: Phone number or SIP URI
- **State**: Current state with visual indicator
- **Intent**: Classified intent with icon
- **Confidence**: Score with color indicator:
  - 🔴 Low (< 60%)
  - 🟡 Medium (60-80%)
  - 🟢 High (> 80%)
- **Duration**: MM:SS format

## Managing Calls

### Selecting a Call

1. Click on any call in the left panel
2. The call becomes highlighted
3. Transcript and info panels update

### Accepting a Transfer

When a transfer is requested:

1. A notification appears at the top right
2. The call entry flashes yellow
3. **Option A**: Click the **Accept** button (bottom)
4. **Option B**: Press **F1** key
5. Call state changes to `LIVE_AGENT`
6. Dashboard shows "In progress" banner

### Rejecting a Transfer

1. When transfer request appears, click **Reject** or press **F2**
2. Call returns to LLM routing
3. Dashboard shows brief "Transfer rejected" message

### Terminating a Call

1. Select the call to terminate
2. Click **Terminate** button or press **F3**
3. Confirmation dialog appears (if enabled)
4. Call state changes to `TERMINATED`
5. Call disappears from list after cleanup

## Keyboard Shortcuts

| Key | Action | When |
|-----|--------|------|
| **F1** | Accept transfer | Transfer dialog shown |
| **F2** | Reject transfer | Transfer dialog shown |
| **F3** | Terminate call | Any time |
| **ESC** | Close dialog / Cancel | Dialog open |
| **Tab** | Move focus between panels | Any time |
| **↑ / ↓** | Navigate call list | Call list focused |
| **Enter** | Select call / Confirm dialog | Any time |
| **Space** | Toggle selection | Call list |

## Notifications

The dashboard shows pop-up notifications:

- **New Call**: "New call from +1234567890"
- **Transfer Request**: "Transfer from billing department"
- **Connection Lost**: "Disconnected - retrying..."
- **Error**: "Error: Connection timeout"

Notifications auto-dismiss after 5 seconds.

## Connection Status

The top-right corner shows connection status:

| Status | Color | Meaning |
|--------|-------|---------|
| Connected | Green | WebSocket active |
| Disconnected | Red | Connection lost, retrying |
| Connecting | Yellow | Initial connection |

## Color Legend

### Call States

| State | Color | Icon |
|-------|-------|------|
| INCOMING | Blue | 📞 |
| LLM_ROUTING | Green | 🤖 |
| LIVE_AGENT | Purple | 👤 |
| TRANSFERRING | Yellow | ⏳ |
| ON_HOLD | Orange | ⏸️ |
| TERMINATED | Red | ❌ |

### Confidence Levels

| Level | Range | Color |
|-------|-------|-------|
| High | > 80% | Green |
| Medium | 60-80% | Yellow |
| Low | < 60% | Red |

## Mobile Support

The dashboard is responsive and works on tablets:

- **Layout**: Stacks panels vertically on narrow screens
- **Touch**: Tap to select, swipe to scroll
- **Keyboard**: External keyboard shortcuts work on tablets with keyboards

## Browser Compatibility

| Browser | Version | Support |
|---------|---------|---------|
| Chrome | 90+ | Full |
| Firefox | 88+ | Full |
| Safari | 14+ | Full |
| Edge | 90+ | Full |
| Mobile Chrome | 90+ | Partial (responsive) |
| Mobile Safari | 14+ | Partial (responsive) |

**Required Features:**
- WebSocket support
- CSS Grid and Flexbox
- JavaScript ES6+

## Troubleshooting

### Page Not Loading

```
1. Check web server is running: curl http://localhost:8081
2. Verify files exist: ls agent-web/web/main.dart.js
3. Check firewall: Port 8081 must be open
4. Try different port: python3 -m http.server 9000
```

### WebSocket Connection Failing

```
1. Verify server is running: curl http://localhost:8080/health
2. Check browser console for errors (F12 → Console tab)
3. Verify WebSocket URL: ws://localhost:8080/ws
4. Check CORS settings in config.yaml
```

### Dashboard Not Updating

```
1. Check connection status in top-right corner
2. Refresh the page (F5)
3. Verify agent is logged in
4. Check browser console for JavaScript errors
```

### Performance Issues

```
1. Close unnecessary browser tabs
2. Disable browser extensions
3. Use Chrome for best performance
4. Check system resources (RAM, CPU)
```

## Customization

### Changing the Theme

Edit `agent-web/web/index.html` CSS:

```css
/* Dark theme (default) */
body {
  background: #1a1a2e;
  color: #eee;
}

/* Light theme */
body {
  background: #f5f5f5;
  color: #333;
}
```

### Adding Custom Branding

1. Replace the header logo in HTML
2. Update colors in CSS
3. Add company-specific fields in call info

## Security Considerations

- **HTTPS**: Use HTTPS in production (WebSocket will use wss://)
- **Authentication**: Enable agent authentication in config.yaml
- **CORS**: Restrict allowed origins in production
- **Session Timeout**: Agents auto-logout after 30 minutes

## Keyboard Shortcut Reference

```
┌─────────────────────────────────────┐
│ Keyboard Shortcuts                   │
├─────────────────────────────────────┤
│ F1  - Accept transfer request        │
│ F2  - Reject transfer request        │
│ F3  - Terminate selected call        │
│ ESC - Close dialog / Cancel          │
│ Tab - Navigate between panels        │
│ ↑/↓ - Scroll call list               │
│ Enter - Select / Confirm            │
└─────────────────────────────────────┘
```
