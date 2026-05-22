package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"go.uber.org/zap"
	
	"voip-server/internal/audio"
	"voip-server/internal/state"
	"voip-server/internal/types"
)

// Server manages WebSocket connections
type Server struct {
	audioPipeline *audio.Pipeline
	callManager   *state.CallManager
	logger        *zap.Logger
	clients       map[string]*Client
	mutex         sync.RWMutex
}

// Client represents a WebSocket client
type Client struct {
	ID         string
	SessionID  string
	Conn       *websocket.Conn
	Send       chan []byte
	Server     *Server
	IsAgent    bool
	AgentID    string
}

// Message represents a WebSocket message
type Message struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

// NewServer creates a new WebSocket server
func NewServer(audioPipeline *audio.Pipeline, callManager *state.CallManager, logger *zap.Logger) *Server {
	return &Server{
		audioPipeline: audioPipeline,
		callManager:   callManager,
		logger:        logger,
		clients:       make(map[string]*Client),
	}
}

// SetupRoutes sets up WebSocket routes
func (s *Server) SetupRoutes(app *fiber.App) {
	// WebSocket endpoint using Fiber's websocket
	app.Use("/ws", func(c *fiber.Ctx) error {
		// IsWebSocketUpgrade returns true if the client requested upgrade to the WebSocket protocol
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	
	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		s.handleConnection(c)
	}))
}

// handleConnection handles a WebSocket connection
func (s *Server) handleConnection(conn *websocket.Conn) {
	client := &Client{
		ID:        generateClientID(),
		Conn:      conn,
		Send:      make(chan []byte, 256),
		Server:    s,
		IsAgent:   false,
	}

	s.addClient(client)
	
	s.logger.Info("WebSocket client connected",
		zap.String("client_id", client.ID))

	// Start goroutines
	go client.readPump()
	go client.writePump()
	
	// Wait for connection to close
	<-client.Send
	
	s.removeClient(client)
}

// readPump handles incoming messages
func (c *Client) readPump() {
	defer func() {
		c.Server.removeClient(c)
		c.Conn.Close()
	}()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.Server.logger.Error("WebSocket error", zap.Error(err))
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			c.Server.logger.Error("Failed to unmarshal message", zap.Error(err))
			continue
		}

		c.handleMessage(msg)
	}
}

// writePump handles outgoing messages
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			c.Conn.WriteMessage(websocket.TextMessage, message)

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming messages
func (c *Client) handleMessage(msg Message) {
	switch msg.Type {
	case "audio_data":
		c.handleAudioData(msg.Payload)
	case "join_call":
		c.handleJoinCall(msg.Payload)
	case "leave_call":
		c.handleLeaveCall(msg.Payload)
	case "agent_login":
		c.handleAgentLogin(msg.Payload)
	case "agent_logout":
		c.handleAgentLogout(msg.Payload)
	case "accept_transfer":
		c.handleAcceptTransfer(msg.Payload)
	case "reject_transfer":
		c.handleRejectTransfer(msg.Payload)
	case "subscribe_calls":
		c.handleSubscribeCalls(msg.Payload)
	default:
		c.Server.logger.Warn("Unknown message type", zap.String("type", msg.Type))
	}
}

// handleAudioData processes incoming audio data
func (c *Client) handleAudioData(payload map[string]interface{}) {
	if c.SessionID == "" {
		c.sendError("Not in a call")
		return
	}

	audioData, ok := payload["data"].(string)
	if !ok {
		c.sendError("Invalid audio data")
		return
	}

	// Decode base64 audio data
	// In production, use proper base64 decoding
	data := []byte(audioData)

	// Send to audio pipeline
	if err := c.Server.audioPipeline.WriteAudio(c.SessionID, data); err != nil {
		c.sendError(err.Error())
	}
}

// handleJoinCall handles joining a call
func (c *Client) handleJoinCall(payload map[string]interface{}) {
	sessionID, ok := payload["session_id"].(string)
	if !ok {
		c.sendError("Missing session_id")
		return
	}

	c.SessionID = sessionID
	
	// Start sending audio from pipeline
	go c.streamAudio()

	c.sendMessage("joined_call", map[string]interface{}{
		"session_id": sessionID,
	})
}

// handleLeaveCall handles leaving a call
func (c *Client) handleLeaveCall(payload map[string]interface{}) {
	if c.SessionID != "" {
		c.SessionID = ""
	}

	c.sendMessage("left_call", map[string]interface{}{})
}

// handleAgentLogin handles agent login
func (c *Client) handleAgentLogin(payload map[string]interface{}) {
	agentID, ok := payload["agent_id"].(string)
	if !ok {
		c.sendError("Missing agent_id")
		return
	}

	c.IsAgent = true
	c.AgentID = agentID

	c.sendMessage("agent_logged_in", map[string]interface{}{
		"agent_id": agentID,
	})

	// Send current active calls
	calls := c.Server.callManager.GetActiveSessions()
	callData := make([]map[string]interface{}, 0, len(calls))
	for _, call := range calls {
		callData = append(callData, map[string]interface{}{
			"id":           call.ID,
			"state":        call.State,
			"caller_id":    call.CallerID,
			"duration":     call.Duration,
			"intent":       call.Intent,
			"confidence":   call.Confidence,
		})
	}

	c.sendMessage("active_calls", map[string]interface{}{
		"calls": callData,
	})
}

// handleAgentLogout handles agent logout
func (c *Client) handleAgentLogout(payload map[string]interface{}) {
	c.IsAgent = false
	c.AgentID = ""

	c.sendMessage("agent_logged_out", map[string]interface{}{})
}

// handleAcceptTransfer handles accepting a transfer
func (c *Client) handleAcceptTransfer(payload map[string]interface{}) {
	sessionID, ok := payload["session_id"].(string)
	if !ok {
		c.sendError("Missing session_id")
		return
	}

	// Update call session
	ctx := context.Background()
	if err := c.Server.callManager.SetTransferData(ctx, sessionID, c.AgentID, "", "accepted"); err != nil {
		c.sendError(err.Error())
		return
	}

	if err := c.Server.callManager.UpdateState(ctx, sessionID, types.StateLiveAgent); err != nil {
		c.sendError(err.Error())
		return
	}

	c.sendMessage("transfer_accepted", map[string]interface{}{
		"session_id": sessionID,
	})
}

// handleRejectTransfer handles rejecting a transfer
func (c *Client) handleRejectTransfer(payload map[string]interface{}) {
	sessionID, ok := payload["session_id"].(string)
	if !ok {
		c.sendError("Missing session_id")
		return
	}

	c.sendMessage("transfer_rejected", map[string]interface{}{
		"session_id": sessionID,
	})
}

// handleSubscribeCalls handles subscribing to call events
func (c *Client) handleSubscribeCalls(payload map[string]interface{}) {
	// Client is now subscribed to call events
	c.sendMessage("subscribed", map[string]interface{}{})
}

// streamAudio streams audio from the pipeline
func (c *Client) streamAudio() {
	for c.SessionID != "" {
		data, err := c.Server.audioPipeline.ReadAudio(c.SessionID)
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// Encode audio data
		c.sendMessage("audio_data", map[string]interface{}{
			"session_id": c.SessionID,
			"data":       string(data), // In production, use base64
		})
	}
}

// sendMessage sends a message to the client
func (c *Client) sendMessage(msgType string, payload map[string]interface{}) {
	msg := Message{
		Type:    msgType,
		Payload: payload,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		c.Server.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}

	select {
	case c.Send <- data:
	default:
		c.Server.logger.Warn("Client send buffer full", zap.String("client_id", c.ID))
	}
}

// sendError sends an error message
func (c *Client) sendError(message string) {
	c.sendMessage("error", map[string]interface{}{
		"message": message,
	})
}

// addClient adds a client
func (s *Server) addClient(client *Client) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.clients[client.ID] = client
}

// removeClient removes a client
func (s *Server) removeClient(client *Client) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.clients, client.ID)
	close(client.Send)
}

// BroadcastToAgents broadcasts a message to all agent clients
func (s *Server) BroadcastToAgents(msgType string, payload map[string]interface{}) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	data, err := json.Marshal(Message{
		Type:    msgType,
		Payload: payload,
	})
	if err != nil {
		s.logger.Error("Failed to marshal broadcast message", zap.Error(err))
		return
	}

	for _, client := range s.clients {
		if client.IsAgent {
			select {
			case client.Send <- data:
			default:
			}
		}
	}
}

// BroadcastCallEvent broadcasts a call event to subscribed agents
func (s *Server) BroadcastCallEvent(eventType string, sessionID string, data map[string]interface{}) {
	payload := map[string]interface{}{
		"session_id": sessionID,
		"timestamp":  time.Now().Unix(),
	}
	
	for k, v := range data {
		payload[k] = v
	}

	s.BroadcastToAgents(eventType, payload)
}

// generateClientID generates a unique client ID
func generateClientID() string {
	return fmt.Sprintf("client_%d", time.Now().UnixNano())
}
