package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gorilla/websocket"
	"github.com/rivo/tview"
)

// AgentTUI represents the terminal UI agent application
type AgentTUI struct {
	app           *tview.Application
	client        *WebSocketClient
	pages         *tview.Pages
	
	// UI components
	callList      *tview.List
	transcript    *tview.TextView
	info          *tview.TextView
	statusBar     *tview.TextView
	
	// State
	agentID       string
	connected     bool
	selectedCall  string
	calls         map[string]*CallInfo
	
	// Channels
	recvChan      chan Message
	sendChan      chan Message
}

// CallInfo represents call information
type CallInfo struct {
	ID         string
	State      string
	CallerID   string
	Duration   int
	Intent     string
	Confidence float64
	Transcript []TranscriptEntry
	Timestamp  time.Time
}

// TranscriptEntry represents a transcript entry
type TranscriptEntry struct {
	Speaker string
	Text    string
	Time    time.Time
}

// WebSocketClient represents the WebSocket client
type WebSocketClient struct {
	conn     *websocket.Conn
	url      string
	agentID  string
	recvChan chan Message
	sendChan chan Message
	closed   bool
}

// Message represents a WebSocket message
type Message struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

func main() {
	// Get WebSocket URL from environment or use default
	wsURL := os.Getenv("WS_URL")
	if wsURL == "" {
		wsURL = "ws://localhost:8080/ws"
	}

	// Get agent ID
	agentID := os.Getenv("AGENT_ID")
	if agentID == "" {
		agentID = fmt.Sprintf("agent_%d", time.Now().Unix())
	}

	tui := NewAgentTUI(wsURL, agentID)
	
	if err := tui.Run(); err != nil {
		log.Fatalf("Error running TUI: %v", err)
	}
}

// NewAgentTUI creates a new TUI agent
func NewAgentTUI(wsURL, agentID string) *AgentTUI {
	tui := &AgentTUI{
		app:      tview.NewApplication(),
		agentID:  agentID,
		calls:    make(map[string]*CallInfo),
		recvChan: make(chan Message, 100),
		sendChan: make(chan Message, 100),
	}

	tui.client = NewWebSocketClient(wsURL, agentID, tui.recvChan, tui.sendChan)
	tui.setupUI()
	
	return tui
}

// setupUI sets up the terminal UI
func (t *AgentTUI) setupUI() {
	t.app.SetInputCapture(t.handleInput)

	// Create call list
	t.callList = tview.NewList()
	t.callList.SetBorder(true)
	t.callList.SetTitle(" Active Calls ")
	t.callList.SetSelectedFunc(t.onCallSelected)
	t.callList.SetMainTextStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite))
	t.callList.SetSecondaryTextStyle(tcell.StyleDefault.Foreground(tcell.ColorGray))
	
	// Highlight selected
	t.callList.SetSelectedTextColor(tcell.ColorBlack)
	t.callList.SetSelectedBackgroundColor(tcell.ColorGreen)

	// Create transcript view
	t.transcript = tview.NewTextView()
	t.transcript.SetBorder(true)
	t.transcript.SetTitle(" Transcript ")
	t.transcript.SetDynamicColors(true)
	t.transcript.SetScrollable(true)
	t.transcript.SetWrap(true)

	// Create info panel
	t.info = tview.NewTextView()
	t.info.SetBorder(true)
	t.info.SetTitle(" Call Info ")
	t.info.SetDynamicColors(true)

	// Create status bar
	t.statusBar = tview.NewTextView()
	t.statusBar.SetTextAlign(tview.AlignLeft)
	t.statusBar.SetDynamicColors(true)
	t.updateStatusBar()

	// Layout
	// Left side: call list
	// Right side: transcript (top) and info (bottom)
	// Bottom: status bar
	
	rightPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.transcript, 0, 2, false).
		AddItem(t.info, 8, 1, false)

	mainLayout := tview.NewFlex().
		AddItem(t.callList, 40, 1, true).
		AddItem(rightPanel, 0, 2, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(mainLayout, 0, 1, true).
		AddItem(t.statusBar, 1, 0, false)

	t.pages = tview.NewPages()
	t.pages.AddPage("main", layout, true, true)

	t.app.SetRoot(t.pages, true)
}

// Run starts the TUI
func (t *AgentTUI) Run() error {
	// Connect to WebSocket
	if err := t.client.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	t.connected = true
	
	// Login as agent
	t.client.Send(Message{
		Type: "agent_login",
		Payload: map[string]interface{}{
			"agent_id": t.agentID,
		},
	})

	// Subscribe to call events
	t.client.Send(Message{
		Type: "subscribe_calls",
		Payload: map[string]interface{}{},
	})

	// Start message handler
	go t.handleMessages()

	// Refresh UI periodically
	go t.refreshUI()

	return t.app.Run()
}

// handleMessages handles incoming WebSocket messages
func (t *AgentTUI) handleMessages() {
	for msg := range t.client.recvChan {
		switch msg.Type {
		case "active_calls":
			t.handleActiveCalls(msg.Payload)
		case "call_started":
			t.handleCallStarted(msg.Payload)
		case "call_ended":
			t.handleCallEnded(msg.Payload)
		case "transcript_update":
			t.handleTranscriptUpdate(msg.Payload)
		case "transfer_requested":
			t.handleTransferRequest(msg.Payload)
		case "state_change":
			t.handleStateChange(msg.Payload)
		case "error":
			t.handleError(msg.Payload)
		}

		// Queue UI update
		t.app.QueueUpdateDraw(func() {})
	}
}

// handleActiveCalls handles active calls list
func (t *AgentTUI) handleActiveCalls(payload map[string]interface{}) {
	calls, ok := payload["calls"].([]interface{})
	if !ok {
		return
	}

	for _, c := range calls {
		callData, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		call := t.parseCallData(callData)
		t.calls[call.ID] = call
	}

	t.updateCallList()
}

// handleCallStarted handles new call started
func (t *AgentTUI) handleCallStarted(payload map[string]interface{}) {
	call := t.parseCallData(payload)
	t.calls[call.ID] = call
	
	// Alert user
	t.app.QueueUpdateDraw(func() {
		t.updateCallList()
		t.showAlert(fmt.Sprintf("New call: %s", call.CallerID))
	})
}

// handleCallEnded handles call ended
func (t *AgentTUI) handleCallEnded(payload map[string]interface{}) {
	sessionID, _ := payload["session_id"].(string)
	if sessionID != "" {
		delete(t.calls, sessionID)
		
		if t.selectedCall == sessionID {
			t.selectedCall = ""
			t.updateTranscript()
			t.updateInfo()
		}
	}
	
	t.app.QueueUpdateDraw(func() {
		t.updateCallList()
	})
}

// handleTranscriptUpdate handles transcript updates
func (t *AgentTUI) handleTranscriptUpdate(payload map[string]interface{}) {
	sessionID, _ := payload["session_id"].(string)
	speaker, _ := payload["speaker"].(string)
	text, _ := payload["text"].(string)

	call, exists := t.calls[sessionID]
	if !exists {
		return
	}

	call.Transcript = append(call.Transcript, TranscriptEntry{
		Speaker: speaker,
		Text:    text,
		Time:    time.Now(),
	})

	if t.selectedCall == sessionID {
		t.app.QueueUpdateDraw(func() {
			t.updateTranscript()
		})
	}
}

// handleTransferRequest handles transfer requests
func (t *AgentTUI) handleTransferRequest(payload map[string]interface{}) {
	sessionID, _ := payload["session_id"].(string)
	reason, _ := payload["reason"].(string)
	intent, _ := payload["intent"].(string)

	call, exists := t.calls[sessionID]
	if !exists {
		return
	}

	call.Intent = intent
	
	t.app.QueueUpdateDraw(func() {
		t.showTransferDialog(sessionID, call.CallerID, reason)
	})
}

// handleStateChange handles state changes
func (t *AgentTUI) handleStateChange(payload map[string]interface{}) {
	sessionID, _ := payload["session_id"].(string)
	newState, _ := payload["new_state"].(string)

	call, exists := t.calls[sessionID]
	if !exists {
		return
	}

	call.State = newState
	
	t.app.QueueUpdateDraw(func() {
		t.updateCallList()
		t.updateInfo()
	})
}

// handleError handles errors
func (t *AgentTUI) handleError(payload map[string]interface{}) {
	message, _ := payload["message"].(string)
	t.showAlert(fmt.Sprintf("Error: %s", message))
}

// parseCallData parses call data from payload
func (t *AgentTUI) parseCallData(data map[string]interface{}) *CallInfo {
	id, _ := data["id"].(string)
	state, _ := data["state"].(string)
	callerID, _ := data["caller_id"].(string)
	
	duration := 0
	if d, ok := data["duration"].(float64); ok {
		duration = int(d)
	}
	
	intent, _ := data["intent"].(string)
	
	confidence := 0.0
	if c, ok := data["confidence"].(float64); ok {
		confidence = c
	}

	return &CallInfo{
		ID:         id,
		State:      state,
		CallerID:   callerID,
		Duration:   duration,
		Intent:     intent,
		Confidence: confidence,
		Timestamp:  time.Now(),
		Transcript: make([]TranscriptEntry, 0),
	}
}

// updateCallList updates the call list
func (t *AgentTUI) updateCallList() {
	t.callList.Clear()

	for _, call := range t.calls {
		status := "[green]●"
		if call.State == "TRANSFERRING" {
			status = "[yellow]◐"
		} else if call.State == "TERMINATED" {
			status = "[red]○"
		}

		mainText := fmt.Sprintf("%s %s", status, call.CallerID)
		secondaryText := fmt.Sprintf("State: %s | Intent: %s | Dur: %ds", 
			call.State, call.Intent, call.Duration)
		
		t.callList.AddItem(mainText, secondaryText, 0, nil)
	}

	if len(t.calls) == 0 {
		t.callList.AddItem("No active calls", "", 0, nil)
	}
}

// updateTranscript updates the transcript view
func (t *AgentTUI) updateTranscript() {
	t.transcript.Clear()

	if t.selectedCall == "" {
		fmt.Fprint(t.transcript, "Select a call to view transcript")
		return
	}

	call, exists := t.calls[t.selectedCall]
	if !exists {
		fmt.Fprint(t.transcript, "Call not found")
		return
	}

	for _, entry := range call.Transcript {
		color := "white"
		if entry.Speaker == "caller" {
			color = "cyan"
		} else if entry.Speaker == "llm" {
			color = "green"
		} else if entry.Speaker == "agent" {
			color = "yellow"
		}

		fmt.Fprintf(t.transcript, "[%s]%s:[-] %s\n\n", color, entry.Speaker, entry.Text)
	}
}

// updateInfo updates the info panel
func (t *AgentTUI) updateInfo() {
	t.info.Clear()

	if t.selectedCall == "" {
		fmt.Fprint(t.info, "Select a call to view details")
		return
	}

	call, exists := t.calls[t.selectedCall]
	if !exists {
		fmt.Fprint(t.info, "Call not found")
		return
	}

	confidenceColor := "green"
	if call.Confidence < 0.6 {
		confidenceColor = "red"
	} else if call.Confidence < 0.8 {
		confidenceColor = "yellow"
	}

	info := fmt.Sprintf(`[yellow]Call ID:[-] %s
[yellow]Caller:[-] %s
[yellow]State:[-] %s
[yellow]Intent:[-] %s
[yellow]Confidence:[-] [%s]%.2f[-]
[yellow]Duration:[-] %ds

[yellow]Shortcuts:[-]
F1: Accept Transfer  F2: Reject  F3: Terminate`,
		call.ID, call.CallerID, call.State, call.Intent,
		confidenceColor, call.Confidence, call.Duration)

	fmt.Fprint(t.info, info)
}

// updateStatusBar updates the status bar
func (t *AgentTUI) updateStatusBar() {
	status := "[green]Connected"
	if !t.connected {
		status = "[red]Disconnected"
	}

	statusText := fmt.Sprintf(" %s | Agent: %s | Calls: %d | Press F10 for help", 
		status, t.agentID, len(t.calls))
	
	t.statusBar.SetText(statusText)
}

// onCallSelected handles call selection
func (t *AgentTUI) onCallSelected(index int, mainText string, secondaryText string, shortcut rune) {
	// Find call by index
	idx := 0
	for id := range t.calls {
		if idx == index {
			t.selectedCall = id
			t.updateTranscript()
			t.updateInfo()
			return
		}
		idx++
	}
}

// handleInput handles keyboard input
func (t *AgentTUI) handleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyF1:
		t.acceptTransfer()
		return nil
	case tcell.KeyF2:
		t.rejectTransfer()
		return nil
	case tcell.KeyF3:
		t.terminateCall()
		return nil
	case tcell.KeyF10:
		t.showHelp()
		return nil
	case tcell.KeyEscape:
		t.app.Stop()
		return nil
	}

	return event
}

// acceptTransfer accepts a transfer
func (t *AgentTUI) acceptTransfer() {
	if t.selectedCall == "" {
		t.showAlert("No call selected")
		return
	}

	t.client.Send(Message{
		Type: "accept_transfer",
		Payload: map[string]interface{}{
			"session_id": t.selectedCall,
		},
	})
}

// rejectTransfer rejects a transfer
func (t *AgentTUI) rejectTransfer() {
	if t.selectedCall == "" {
		t.showAlert("No call selected")
		return
	}

	t.client.Send(Message{
		Type: "reject_transfer",
		Payload: map[string]interface{}{
			"session_id": t.selectedCall,
		},
	})
}

// terminateCall terminates a call
func (t *AgentTUI) terminateCall() {
	if t.selectedCall == "" {
		t.showAlert("No call selected")
		return
	}

	t.client.Send(Message{
		Type: "terminate_call",
		Payload: map[string]interface{}{
			"session_id": t.selectedCall,
		},
	})
}

// showAlert shows an alert
func (t *AgentTUI) showAlert(message string) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			t.pages.RemovePage("alert")
		})

	t.pages.AddPage("alert", modal, false, true)
}

// showHelp shows help dialog
func (t *AgentTUI) showHelp() {
	help := `
Keyboard Shortcuts:
  F1  - Accept transfer request
  F2  - Reject transfer request
  F3  - Terminate selected call
  F10 - Show this help
  Esc - Quit application

Navigation:
  Tab/Arrow keys - Navigate between panels
  Enter          - Select call
`
	modal := tview.NewModal().
		SetText(help).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			t.pages.RemovePage("help")
		})

	t.pages.AddPage("help", modal, true, true)
}

// showTransferDialog shows transfer dialog
func (t *AgentTUI) showTransferDialog(sessionID, callerID, reason string) {
	text := fmt.Sprintf("Transfer request for %s\nReason: %s", callerID, reason)
	
	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{"Accept", "Reject"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Accept" {
				t.selectedCall = sessionID
				t.acceptTransfer()
			} else {
				t.rejectTransfer()
			}
			t.pages.RemovePage("transfer")
		})

	t.pages.AddPage("transfer", modal, true, true)
}

// refreshUI periodically refreshes the UI
func (t *AgentTUI) refreshUI() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Update durations
		for _, call := range t.calls {
			call.Duration++
		}

		t.app.QueueUpdateDraw(func() {
			t.updateStatusBar()
			t.updateCallList()
			t.updateInfo()
		})
	}
}

// NewWebSocketClient creates a new WebSocket client
func NewWebSocketClient(url, agentID string, recvChan, sendChan chan Message) *WebSocketClient {
	return &WebSocketClient{
		url:      url,
		agentID:  agentID,
		recvChan: recvChan,
		sendChan: sendChan,
	}
}

// Connect connects to the WebSocket server
func (c *WebSocketClient) Connect() error {
	conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
	if err != nil {
		return err
	}

	c.conn = conn

	// Start receive goroutine
	go c.receive()

	// Start send goroutine
	go c.send()

	return nil
}

// receive receives messages from the server
func (c *WebSocketClient) receive() {
	for {
		if c.closed {
			return
		}

		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if !c.closed {
				log.Printf("WebSocket read error: %v", err)
			}
			return
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			continue
		}

		c.recvChan <- msg
	}
}

// send sends messages to the server
func (c *WebSocketClient) send() {
	for msg := range c.sendChan {
		if c.closed || c.conn == nil {
			return
		}

		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("Failed to marshal message: %v", err)
			continue
		}

		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("WebSocket write error: %v", err)
			return
		}
	}
}

// Send sends a message
func (c *WebSocketClient) Send(msg Message) {
	select {
	case c.sendChan <- msg:
	default:
		log.Println("Send channel full")
	}
}

// Close closes the connection
func (c *WebSocketClient) Close() {
	c.closed = true
	if c.conn != nil {
		c.conn.Close()
	}
}
