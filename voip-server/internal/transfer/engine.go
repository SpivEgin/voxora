package transfer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"voip-server/internal/config"
	"voip-server/internal/llm"
	"voip-server/internal/state"
	"voip-server/internal/types"
)

// Engine evaluates transfer conditions and executes transfers
type Engine struct {
	config      *config.TransferConfig
	callManager *state.CallManager
	logger      *zap.Logger
	transfers   chan TransferRequest
}

// TransferRequest represents a transfer request
type TransferRequest struct {
	SessionID      string
	Target         config.TransferTarget
	Reason         string
	Confidence     float64
	Intent         string
	Timestamp      time.Time
}

// TransferResult represents the result of a transfer
type TransferResult struct {
	Success   bool
	SessionID string
	Target    string
	Error     error
}

// ConditionResult contains the result of condition evaluation
type ConditionResult struct {
	ShouldTransfer bool
	Reason         string
	Target         *config.TransferTarget
}

// NewEngine creates a new transfer engine
func NewEngine(cfg *config.TransferConfig, callManager *state.CallManager, logger *zap.Logger) *Engine {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Engine{
		config:      cfg,
		callManager: callManager,
		logger:      logger,
		transfers:   make(chan TransferRequest, 100),
	}
}

// Start starts the transfer engine
func (e *Engine) Start(ctx context.Context) {
	go e.processTransfers(ctx)
}

// Stop stops the transfer engine
func (e *Engine) Stop() {
	close(e.transfers)
}

// Evaluate evaluates transfer conditions for a call
func (e *Engine) Evaluate(ctx context.Context, sessionID string, llmResult *llm.CompletionResult, transcript []string) (*ConditionResult, error) {
	session, err := e.callManager.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	result := &ConditionResult{
		ShouldTransfer: false,
	}

	// Check if transfer is disabled
	if !e.config.Enabled {
		return result, nil
	}

	// 1. Check caller explicitly requests human
	if e.config.Conditions.CallerRequestsHuman {
		if e.checkCallerRequestsHuman(transcript) {
			result.ShouldTransfer = true
			result.Reason = "caller_requested_human"
			result.Target = e.findBestTarget("support")
			e.logger.Info("Transfer triggered: caller requested human",
				zap.String("session_id", sessionID))
			return result, nil
		}
	}

	// 2. Check LLM confidence threshold
	if llmResult.Confidence < e.config.Conditions.ConfidenceThreshold {
		result.ShouldTransfer = true
		result.Reason = fmt.Sprintf("low_confidence: %.2f", llmResult.Confidence)
		result.Target = e.findBestTarget(llmResult.Intent)
		e.logger.Info("Transfer triggered: low LLM confidence",
			zap.String("session_id", sessionID),
			zap.Float64("confidence", llmResult.Confidence))
		return result, nil
	}

	// 3. Check intent triggers
	if e.config.Conditions.IntentTriggers != nil {
		for _, trigger := range e.config.Conditions.IntentTriggers {
			if strings.EqualFold(llmResult.Intent, trigger) || 
			   strings.Contains(strings.ToLower(llmResult.Intent), strings.ToLower(trigger)) {
				result.ShouldTransfer = true
				result.Reason = fmt.Sprintf("intent_trigger: %s", trigger)
				result.Target = e.findBestTarget(trigger)
				e.logger.Info("Transfer triggered: intent match",
					zap.String("session_id", sessionID),
					zap.String("intent", llmResult.Intent),
					zap.String("trigger", trigger))
				return result, nil
			}
		}
	}

	// 4. Check LLM requests transfer
	if llmResult.TransferToAgent {
		result.ShouldTransfer = true
		result.Reason = fmt.Sprintf("llm_request: %s", llmResult.Reason)
		result.Target = e.findBestTarget(llmResult.Intent)
		e.logger.Info("Transfer triggered: LLM requested transfer",
			zap.String("session_id", sessionID),
			zap.String("reason", llmResult.Reason))
		return result, nil
	}

	// 5. Check max LLM turns
	if session.LLMTurnCount >= e.config.Conditions.MaxLLMTurns {
		result.ShouldTransfer = true
		result.Reason = fmt.Sprintf("max_turns_exceeded: %d", session.LLMTurnCount)
		result.Target = e.findBestTarget(llmResult.Intent)
		e.logger.Info("Transfer triggered: max LLM turns exceeded",
			zap.String("session_id", sessionID),
			zap.Int("turns", session.LLMTurnCount))
		return result, nil
	}

	// 6. Check call duration
	duration := time.Since(session.StartTime).Seconds()
	if int(duration) >= e.config.Conditions.MaxCallDuration {
		result.ShouldTransfer = true
		result.Reason = fmt.Sprintf("max_duration_exceeded: %.0fs", duration)
		result.Target = e.findBestTarget("support")
		e.logger.Info("Transfer triggered: max call duration exceeded",
			zap.String("session_id", sessionID),
			zap.Float64("duration", duration))
		return result, nil
	}

	// 7. Check abuse/profanity
	if e.config.Conditions.AbuseDetection {
		if e.detectAbusiveLanguage(transcript) {
			result.ShouldTransfer = true
			result.Reason = "abusive_language_detected"
			result.Target = e.findBestTarget("supervisor")
			e.logger.Info("Transfer triggered: abusive language detected",
				zap.String("session_id", sessionID))
			return result, nil
		}
	}

	return result, nil
}

// RequestTransfer queues a transfer request
func (e *Engine) RequestTransfer(ctx context.Context, sessionID string, target *config.TransferTarget, reason string) error {
	request := TransferRequest{
		SessionID: sessionID,
		Target:    *target,
		Reason:    reason,
		Timestamp: time.Now(),
	}

	select {
	case e.transfers <- request:
		e.logger.Info("Transfer request queued",
			zap.String("session_id", sessionID),
			zap.String("target", target.Name))
		return nil
	default:
		return fmt.Errorf("transfer queue is full")
	}
}

// processTransfers processes transfer requests
func (e *Engine) processTransfers(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case request, ok := <-e.transfers:
			if !ok {
				return
			}
			
			// Update call state
			if err := e.callManager.UpdateState(ctx, request.SessionID, types.StateTransferring); err != nil {
				e.logger.Error("Failed to update state for transfer",
					zap.String("session_id", request.SessionID),
					zap.Error(err))
				continue
			}

			// Execute transfer
			// Note: Actual SIP REFER would be implemented here
			e.logger.Info("Executing transfer",
				zap.String("session_id", request.SessionID),
				zap.String("target", request.Target.SIPURI),
				zap.String("reason", request.Reason))

			// For now, transition to live agent state
			// In production, this would involve SIP REFER or bridging
			if err := e.callManager.UpdateState(ctx, request.SessionID, types.StateLiveAgent); err != nil {
				e.logger.Error("Failed to transition to live agent",
					zap.String("session_id", request.SessionID),
					zap.Error(err))
				continue
			}

			// Set transfer data
			if err := e.callManager.SetTransferData(ctx, request.SessionID, 
				"", "", request.Reason); err != nil {
				e.logger.Error("Failed to set transfer data",
					zap.String("session_id", request.SessionID),
					zap.Error(err))
			}
		}
	}
}

// checkCallerRequestsHuman checks if caller requested a human agent
func (e *Engine) checkCallerRequestsHuman(transcript []string) bool {
	humanRequests := []string{
		"human", "agent", "person", "representative",
		"operator", "supervisor", "manager", "live person",
		"real person", "talk to someone", "speak to someone",
		"transfer me", "connect me", "i want to talk to",
		"can i speak to", "can i talk to",
	}

	for _, text := range transcript {
		lower := strings.ToLower(text)
		for _, request := range humanRequests {
			if strings.Contains(lower, request) {
				return true
			}
		}
	}

	return false
}

// detectAbusiveLanguage detects abusive language in transcript
func (e *Engine) detectAbusiveLanguage(transcript []string) bool {
	abusiveWords := []string{
		"stupid", "idiot", "moron", "jerk", "asshole",
		"damn", "hell", "crap", "shut up",
	}

	for _, text := range transcript {
		lower := strings.ToLower(text)
		for _, word := range abusiveWords {
			if strings.Contains(lower, word) {
				return true
			}
		}
	}

	return false
}

// findBestTarget finds the best transfer target based on intent
func (e *Engine) findBestTarget(intent string) *config.TransferTarget {
	intentToTarget := map[string]string{
		"billing":     "billing",
		"payment":     "billing",
		"invoice":     "billing",
		"charge":      "billing",
		"refund":      "billing",
		"technical":   "support",
		"support":     "support",
		"help":        "support",
		"problem":     "support",
		"issue":       "support",
		"broken":      "support",
		"sales":       "sales",
		"buy":         "sales",
		"purchase":    "sales",
		"price":       "sales",
		"quote":       "sales",
		"complaint":   "support",
		"angry":       "support",
		"unhappy":     "support",
		"emergency":   "emergency",
		"urgent":      "emergency",
		"supervisor":  "emergency",
	}

	targetName := intentToTarget[strings.ToLower(intent)]
	if targetName == "" {
		targetName = "support" // Default
	}

	// Find matching target
	for i := range e.config.Targets {
		if e.config.Targets[i].Name == targetName {
			return &e.config.Targets[i]
		}
	}

	// Return first target as fallback
	if len(e.config.Targets) > 0 {
		return &e.config.Targets[0]
	}

	return nil
}

// GetTransferTargets returns available transfer targets
func (e *Engine) GetTransferTargets() []config.TransferTarget {
	return e.config.Targets
}

// IsTransferEnabled returns whether transfers are enabled
func (e *Engine) IsTransferEnabled() bool {
	return e.config.Enabled
}
