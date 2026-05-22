package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	
	"voip-server/internal/audio"
	"voip-server/internal/config"
	"voip-server/internal/llm"
	"voip-server/internal/sip"
	"voip-server/internal/state"
	"voip-server/internal/types"
	"voip-server/internal/stt"
	"voip-server/internal/tts"
	"voip-server/internal/transfer"
)

// Handler handles HTTP API requests
type Handler struct {
	app           *fiber.App
	config        *config.ServerConfig
	callManager   *state.CallManager
	sipServer     *sip.Server
	audioPipeline *audio.Pipeline
	sttManager    *stt.Manager
	ttsManager    *tts.Manager
	llmManager    *llm.Manager
	transfer      *transfer.Engine
	logger        *zap.Logger
}

// NewHandler creates a new API handler
func NewHandler(cfg *config.ServerConfig, cm *state.CallManager, ss *sip.Server, 
	ap *audio.Pipeline, stt *stt.Manager, tts *tts.Manager, llm *llm.Manager,
	tr *transfer.Engine, logger *zap.Logger) *Handler {
	return &Handler{
		config:        cfg,
		callManager:   cm,
		sipServer:     ss,
		audioPipeline: ap,
		sttManager:    stt,
		ttsManager:    tts,
		llmManager:    llm,
		transfer:      tr,
		logger:        logger,
	}
}

// SetupRoutes sets up API routes
func (h *Handler) SetupRoutes(app *fiber.App) {
	h.app = app

	// Health check
	app.Get("/health", h.healthCheck)
	app.Get("/ready", h.readinessCheck)

	// API v1 routes
	v1 := app.Group("/api/v1")

	// Calls
	v1.Get("/calls", h.listCalls)
	v1.Get("/calls/:id", h.getCall)
	v1.Post("/calls/:id/terminate", h.terminateCall)
	v1.Post("/calls/:id/transfer", h.transferCall)
	v1.Get("/calls/:id/transcript", h.getTranscript)

	// Agents
	v1.Get("/agents", h.listAgents)
	v1.Get("/agents/:id/calls", h.getAgentCalls)
	v1.Post("/agents/:id/login", h.agentLogin)
	v1.Post("/agents/:id/logout", h.agentLogout)

	// System
	v1.Get("/system/stats", h.getSystemStats)
	v1.Get("/system/health", h.getHealthStatus)
	
	// Transfer targets
	v1.Get("/transfer/targets", h.getTransferTargets)
}

// healthCheck handles health check requests
func (h *Handler) healthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "healthy",
		"time":   time.Now().Unix(),
	})
}

// readinessCheck handles readiness check requests
func (h *Handler) readinessCheck(c *fiber.Ctx) error {
	// Check all services
	checks := map[string]bool{
		"redis": h.callManager != nil,
		"sip":   h.sipServer != nil,
	}

	allReady := true
	for _, ready := range checks {
		if !ready {
			allReady = false
			break
		}
	}

	if !allReady {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "not_ready",
			"checks": checks,
		})
	}

	return c.JSON(fiber.Map{
		"status": "ready",
		"checks": checks,
	})
}

// listCalls lists all calls
func (h *Handler) listCalls(c *fiber.Ctx) error {
	callState := c.Query("state")
	
	var calls []*types.CallSession
	if callState != "" {
		calls = h.callManager.GetSessionsByState(types.CallState(callState))
	} else {
		calls = h.callManager.GetActiveSessions()
	}

	// Build response
	response := make([]map[string]interface{}, 0, len(calls))
	for _, call := range calls {
		response = append(response, map[string]interface{}{
			"id":            call.ID,
			"state":         call.State,
			"caller_id":     call.CallerID,
			"caller_number": call.CallerNumber,
			"called_number": call.CalledNumber,
			"duration":      call.Duration,
			"intent":        call.Intent,
			"confidence":    call.Confidence,
			"agent_id":      call.AgentID,
			"agent_name":    call.AgentName,
		})
	}

	return c.JSON(fiber.Map{
		"calls": response,
		"count": len(response),
	})
}

// getCall gets a specific call
func (h *Handler) getCall(c *fiber.Ctx) error {
	callID := c.Params("id")

	session, err := h.callManager.GetSession(context.Background(), callID)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Call not found",
		})
	}

	return c.JSON(fiber.Map{
		"id":              session.ID,
		"state":           session.State,
		"caller_id":       session.CallerID,
		"caller_number":   session.CallerNumber,
		"called_number":   session.CalledNumber,
		"start_time":      session.StartTime,
		"duration":        session.Duration,
		"llm_turn_count":  session.LLMTurnCount,
		"transcript":      session.Transcript,
		"llm_summary":     session.LLMSummary,
		"transfer_reason": session.TransferReason,
		"intent":          session.Intent,
		"confidence":      session.Confidence,
		"agent_id":        session.AgentID,
		"agent_name":      session.AgentName,
	})
}

// terminateCall terminates a call
func (h *Handler) terminateCall(c *fiber.Ctx) error {
	callID := c.Params("id")

	ctx := context.Background()
	if err := h.callManager.UpdateState(ctx, callID, types.StateTerminated); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if err := h.callManager.CloseSession(ctx, callID); err != nil {
		h.logger.Warn("Failed to close session", zap.Error(err))
	}

	return c.JSON(fiber.Map{
		"status": "terminated",
		"id":     callID,
	})
}

// transferCall initiates a call transfer
func (h *Handler) transferCall(c *fiber.Ctx) error {
	callID := c.Params("id")

	var req struct {
		Target string `json:"target"`
		Reason string `json:"reason"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Find target
	targets := h.transfer.GetTransferTargets()
	var target *config.TransferTarget
	for _, t := range targets {
		if t.Name == req.Target {
			target = &t
			break
		}
	}

	if target == nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid transfer target",
		})
	}

	ctx := context.Background()
	if err := h.transfer.RequestTransfer(ctx, callID, target, req.Reason); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status":   "transfer_requested",
		"id":       callID,
		"target":   target.Name,
		"sip_uri":  target.SIPURI,
	})
}

// getTranscript gets call transcript
func (h *Handler) getTranscript(c *fiber.Ctx) error {
	callID := c.Params("id")

	session, err := h.callManager.GetSession(context.Background(), callID)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Call not found",
		})
	}

	return c.JSON(fiber.Map{
		"id":         callID,
		"transcript": session.Transcript,
	})
}

// listAgents lists all agents
func (h *Handler) listAgents(c *fiber.Ctx) error {
	// In production, query agent database
	agents := []map[string]interface{}{
		{
			"id":       "agent1",
			"name":     "John Doe",
			"status":   "available",
			"calls":    0,
		},
		{
			"id":       "agent2",
			"name":     "Jane Smith",
			"status":   "busy",
			"calls":    1,
		},
	}

	return c.JSON(fiber.Map{
		"agents": agents,
		"count":  len(agents),
	})
}

// getAgentCalls gets calls assigned to an agent
func (h *Handler) getAgentCalls(c *fiber.Ctx) error {
	agentID := c.Params("id")

	calls := h.callManager.GetAgentSessions(agentID)
	
	response := make([]map[string]interface{}, 0, len(calls))
	for _, call := range calls {
		response = append(response, map[string]interface{}{
			"id":         call.ID,
			"state":      call.State,
			"caller_id":  call.CallerID,
			"duration":   call.Duration,
			"intent":     call.Intent,
		})
	}

	return c.JSON(fiber.Map{
		"agent_id": agentID,
		"calls":    response,
		"count":    len(response),
	})
}

// agentLogin handles agent login
func (h *Handler) agentLogin(c *fiber.Ctx) error {
	agentID := c.Params("id")

	// In production, validate credentials and create session

	return c.JSON(fiber.Map{
		"status":   "logged_in",
		"agent_id": agentID,
	})
}

// agentLogout handles agent logout
func (h *Handler) agentLogout(c *fiber.Ctx) error {
	agentID := c.Params("id")

	// In production, invalidate session

	return c.JSON(fiber.Map{
		"status":   "logged_out",
		"agent_id": agentID,
	})
}

// getSystemStats returns system statistics
func (h *Handler) getSystemStats(c *fiber.Ctx) error {
	stats := map[string]interface{}{
		"active_calls":     h.callManager.GetActiveSessions(),
		"audio_sessions":   h.audioPipeline.GetActiveSessions(),
		"sip_calls":        h.sipServer.GetActiveCalls(),
		"timestamp":        time.Now().Unix(),
	}

	return c.JSON(fiber.Map{
		"stats": stats,
	})
}

// getHealthStatus returns detailed health status
func (h *Handler) getHealthStatus(c *fiber.Ctx) error {
	ctx := context.Background()
	
	status := map[string]interface{}{
		"sip":    "healthy",
		"redis":  "healthy",
	}

	// Check STT
	if err := h.sttManager.HealthCheck(ctx); err != nil {
		status["stt"] = "unhealthy: " + err.Error()
	} else {
		status["stt"] = "healthy"
	}

	// Check TTS
	if err := h.ttsManager.HealthCheck(ctx); err != nil {
		status["tts"] = "unhealthy: " + err.Error()
	} else {
		status["tts"] = "healthy"
	}

	// Check LLM
	if err := h.llmManager.HealthCheck(ctx); err != nil {
		status["llm"] = "unhealthy: " + err.Error()
	} else {
		status["llm"] = "healthy"
	}

	return c.JSON(fiber.Map{
		"status": status,
		"time":   time.Now().Unix(),
	})
}

// getTransferTargets returns available transfer targets
func (h *Handler) getTransferTargets(c *fiber.Ctx) error {
	targets := h.transfer.GetTransferTargets()

	return c.JSON(fiber.Map{
		"targets": targets,
	})
}

// WebhookHandlers handles webhook requests
type WebhookHandlers struct {
	callManager *state.CallManager
	logger      *zap.Logger
}

// NewWebhookHandlers creates new webhook handlers
func NewWebhookHandlers(cm *state.CallManager, logger *zap.Logger) *WebhookHandlers {
	return &WebhookHandlers{
		callManager: cm,
		logger:      logger,
	}
}

// HandleTwilio handles Twilio webhooks
func (h *WebhookHandlers) HandleTwilio(c *fiber.Ctx) error {
	// Parse Twilio webhook
	callSID := c.FormValue("CallSid")
	from := c.FormValue("From")
	to := c.FormValue("To")
	callStatus := c.FormValue("CallStatus")

	h.logger.Info("Twilio webhook received",
		zap.String("call_sid", callSID),
		zap.String("from", from),
		zap.String("to", to),
		zap.String("status", callStatus))

	// Create or update call session
	ctx := context.Background()
	
	switch callStatus {
	case "ringing":
		_, err := h.callManager.CreateSession(ctx, callSID, from, to)
		if err != nil {
			h.logger.Error("Failed to create session", zap.Error(err))
		}
	case "completed":
		h.callManager.UpdateState(ctx, callSID, types.StateTerminated)
		h.callManager.CloseSession(ctx, callSID)
	}

	// Return TwiML response
	return c.Type("application/xml").SendString(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Connect>
        <Stream url="ws://localhost:8080/ws" />
    </Connect>
</Response>`)
}

// HandleSignalWire handles SignalWire webhooks
func (h *WebhookHandlers) HandleSignalWire(c *fiber.Ctx) error {
	// Similar to Twilio but for SignalWire
	h.logger.Info("SignalWire webhook received")
	
	return c.JSON(fiber.Map{
		"status": "received",
	})
}
