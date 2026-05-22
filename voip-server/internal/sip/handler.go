package sip

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"go.uber.org/zap"
	
  "voip-server/internal/config"
	"voip-server/internal/state"
	"voip-server/internal/types"
)

// Server represents the SIP server
type Server struct {
	ua           *sipgo.UserAgent
	server       *sipgo.Server
	config       *config.SIPConfig
	callManager  *state.CallManager
	handlers     *Handlers
	logger       *zap.Logger
	mutex        sync.RWMutex
	activeCalls  map[string]*CallContext
}

// CallContext represents the context for an active SIP call
type CallContext struct {
	SessionID    string
	CallID       string
	From         *sip.Uri
	To           *sip.Uri
	Contact      *sip.Uri
	State        types.CallState
	CreatedAt    time.Time
	RemoteAddr   net.Addr
	AudioHandler AudioHandler
	mutex        sync.RWMutex
}

// AudioHandler handles audio streaming
type AudioHandler interface {
	Start(ctx context.Context, sessionID string) error
	Stop() error
	WriteAudio(data []byte) error
	ReadAudio() ([]byte, error)
}

// NewServer creates a new SIP server
func NewServer(cfg *config.SIPConfig, callManager *state.CallManager, logger *zap.Logger) (*Server, error) {
	// Create User Agent
	ua, err := sipgo.NewUA(
		sipgo.WithUserAgent(cfg.UserAgent),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create SIP user agent: %w", err)
	}

	server := &Server{
		ua:          ua,
		config:      cfg,
		callManager: callManager,
		logger:      logger,
		activeCalls: make(map[string]*CallContext),
	}

	// Create SIP server
	sipServer, err := sipgo.NewServer(ua)
	if err != nil {
		return nil, fmt.Errorf("failed to create SIP server: %w", err)
	}
	server.server = sipServer

	// Register handlers
	server.handlers = NewHandlers(server, logger)
	
	// Register SIP methods
	sipServer.OnInvite(server.handlers.handleInvite)
	sipServer.OnAck(server.handlers.handleAck)
	sipServer.OnBye(server.handlers.handleBye)
	sipServer.OnCancel(server.handlers.handleCancel)
	sipServer.OnOptions(server.handlers.handleOptions)
	sipServer.OnRegister(server.handlers.handleRegister)

	return server, nil
}

// Start starts the SIP server
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting SIP server",
		zap.String("host", s.config.Host),
		zap.Int("port", s.config.Port),
	)

	// Start listening
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	
	// Start in a goroutine
	go func() {
		if err := s.server.ListenAndServe(ctx, "udp", addr); err != nil {
			s.logger.Error("SIP server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop stops the SIP server
func (s *Server) Stop() error {
	s.logger.Info("Stopping SIP server")
	
	// Close all active calls
	s.mutex.Lock()
	for callID, ctx := range s.activeCalls {
		if err := s.terminateCall(ctx.SessionID); err != nil {
			s.logger.Error("Failed to terminate call",
				zap.String("call_id", callID),
				zap.Error(err))
		}
	}
	s.activeCalls = make(map[string]*CallContext)
	s.mutex.Unlock()

	return s.server.Close()
}

// GetActiveCalls returns the number of active calls
func (s *Server) GetActiveCalls() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return len(s.activeCalls)
}

// GetCallContext returns a call context by ID
func (s *Server) GetCallContext(callID string) *CallContext {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.activeCalls[callID]
}

// addCallContext adds a new call context
func (s *Server) addCallContext(ctx *CallContext) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.activeCalls[ctx.CallID] = ctx
}

// removeCallContext removes a call context
func (s *Server) removeCallContext(callID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.activeCalls, callID)
}

// terminateCall terminates a call
func (s *Server) terminateCall(sessionID string) error {
	ctx := context.Background()
	
	// Update call state
	if err := s.callManager.UpdateState(ctx, sessionID, types.StateTerminated); err != nil {
		s.logger.Warn("Failed to update call state",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	return nil
}

// Handlers contains SIP message handlers
type Handlers struct {
	server *Server
	logger *zap.Logger
}

// NewHandlers creates new SIP handlers
func NewHandlers(server *Server, logger *zap.Logger) *Handlers {
	return &Handlers{
		server: server,
		logger: logger,
	}
}

// handleInvite handles INVITE requests
func (h *Handlers) handleInvite(req *sip.Request, tx sip.ServerTransaction) {
	h.logger.Debug("Received INVITE",
		zap.String("from", req.From().Address.String()),
		zap.String("to", req.To().Address.String()),
		zap.String("call_id", req.CallID().Value()))

	// Create call context
	callID := req.CallID().Value()
	from := req.From().Address
	to := req.To().Address
	
	ctx := context.Background()
	
	// Create call session
	session, err := h.server.callManager.CreateSession(ctx, 
		from.User, 
		from.User,  // Use user as caller number
		to.User)
	if err != nil {
		h.logger.Error("Failed to create call session", zap.Error(err))
		resp := sip.NewResponseFromRequest(req, 500, "Internal Server Error", nil)
		tx.Respond(resp)
		return
	}

	// Create call context
	callCtx := &CallContext{
		SessionID:  session.ID,
		CallID:     callID,
		From:       &from,
		To:         &to,
		State:      types.StateIncoming,
		CreatedAt:  time.Now(),
	}

	h.server.addCallContext(callCtx)

	// Update state to LLM routing
	if err := h.server.callManager.UpdateState(ctx, session.ID, types.StateLLMRouting); err != nil {
		h.logger.Error("Failed to update call state", zap.Error(err))
	}

	// Send 180 Ringing
	ringing := sip.NewResponseFromRequest(req, 180, "Ringing", nil)
	if err := tx.Respond(ringing); err != nil {
		h.logger.Error("Failed to send 180 Ringing", zap.Error(err))
	}

	// Send 200 OK with SDP
	ok := sip.NewResponseFromRequest(req, 200, "OK", nil)
	
	// Add SDP body for audio
	body := []byte(generateSDP())
	ok.SetBody(body)
	
	if err := tx.Respond(ok); err != nil {
		h.logger.Error("Failed to send 200 OK", zap.Error(err))
		return
	}

	h.logger.Info("Call established",
		zap.String("session_id", session.ID),
		zap.String("call_id", callID))
}

// handleAck handles ACK requests
func (h *Handlers) handleAck(req *sip.Request, tx sip.ServerTransaction) {
	h.logger.Debug("Received ACK",
		zap.String("call_id", req.CallID().Value()))

	// Acknowledgment received, media can flow
}

// handleBye handles BYE requests
func (h *Handlers) handleBye(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()
	h.logger.Info("Received BYE",
		zap.String("call_id", callID))

	// Get call context
	callCtx := h.server.GetCallContext(callID)
	if callCtx != nil {
		// Terminate call
		if err := h.server.terminateCall(callCtx.SessionID); err != nil {
			h.logger.Error("Failed to terminate call", zap.Error(err))
		}
		h.server.removeCallContext(callID)
	}

	// Send 200 OK
	ok := sip.NewResponseFromRequest(req, 200, "OK", nil)
	if err := tx.Respond(ok); err != nil {
		h.logger.Error("Failed to send 200 OK", zap.Error(err))
	}
}

// handleCancel handles CANCEL requests
func (h *Handlers) handleCancel(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()
	h.logger.Info("Received CANCEL",
		zap.String("call_id", callID))

	// Get call context
	callCtx := h.server.GetCallContext(callID)
	if callCtx != nil {
		// Terminate call
		if err := h.server.terminateCall(callCtx.SessionID); err != nil {
			h.logger.Error("Failed to terminate call", zap.Error(err))
		}
		h.server.removeCallContext(callID)
	}

	// Send 200 OK
	ok := sip.NewResponseFromRequest(req, 200, "OK", nil)
	if err := tx.Respond(ok); err != nil {
		h.logger.Error("Failed to send 200 OK", zap.Error(err))
	}
}

// handleOptions handles OPTIONS requests
func (h *Handlers) handleOptions(req *sip.Request, tx sip.ServerTransaction) {
	h.logger.Debug("Received OPTIONS")
	
	// Send 200 OK
	ok := sip.NewResponseFromRequest(req, 200, "OK", nil)
	ok.AppendHeader(sip.NewHeader("Allow", "INVITE, ACK, BYE, CANCEL, OPTIONS"))
	
	if err := tx.Respond(ok); err != nil {
		h.logger.Error("Failed to send 200 OK", zap.Error(err))
	}
}

// handleRegister handles REGISTER requests
func (h *Handlers) handleRegister(req *sip.Request, tx sip.ServerTransaction) {
	h.logger.Debug("Received REGISTER",
		zap.String("from", req.From().Address.String()))

	// For now, accept all registrations
	// In production, validate credentials
	
	// Send 200 OK
	ok := sip.NewResponseFromRequest(req, 200, "OK", nil)
	ok.AppendHeader(sip.NewHeader("Expires", "3600"))
	
	if err := tx.Respond(ok); err != nil {
		h.logger.Error("Failed to send 200 OK", zap.Error(err))
	}
}

// generateSDP generates a simple SDP offer
func generateSDP() string {
	return `v=0
o=- 0 0 IN IP4 127.0.0.1
s=VoIP Call
t=0 0
m=audio 10000 RTP/AVP 0 8 96
c=IN IP4 127.0.0.1
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:96 opus/48000/2
a=sendrecv
`
}

// TransferCall initiates a SIP REFER transfer
func (s *Server) TransferCall(sessionID, targetURI string) error {
	s.logger.Info("Initiating call transfer",
		zap.String("session_id", sessionID),
		zap.String("target", targetURI))

	// Find call context
	var callCtx *CallContext
	s.mutex.RLock()
	for _, ctx := range s.activeCalls {
		if ctx.SessionID == sessionID {
			callCtx = ctx
			break
		}
	}
	s.mutex.RUnlock()

	if callCtx == nil {
		return fmt.Errorf("call session not found: %s", sessionID)
	}

	// Update state
	ctx := context.Background()
	if err := s.callManager.UpdateState(ctx, sessionID, types.StateTransferring); err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	// Send REFER request
	// Note: This is a simplified implementation
	// In production, you'd use sipgo's dialog management
	
	return nil
}

// SendAudio sends audio to a call
func (s *Server) SendAudio(sessionID string, audioData []byte) error {
	// Find call context
	s.mutex.RLock()
	var callCtx *CallContext
	for _, ctx := range s.activeCalls {
		if ctx.SessionID == sessionID {
			callCtx = ctx
			break
		}
	}
	s.mutex.RUnlock()

	if callCtx == nil {
		return fmt.Errorf("call session not found: %s", sessionID)
	}

	// Send via audio handler
	if callCtx.AudioHandler != nil {
		return callCtx.AudioHandler.WriteAudio(audioData)
	}

	return fmt.Errorf("no audio handler for session: %s", sessionID)
}
