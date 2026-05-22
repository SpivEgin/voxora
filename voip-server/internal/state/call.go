package state

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// CallState represents the current state of a call
type CallState string

const (
	StateIncoming    CallState = "INCOMING"
	StateLLMRouting  CallState = "LLM_ROUTING"
	StateLiveAgent   CallState = "LIVE_AGENT"
	StateTerminated  CallState = "TERMINATED"
	StateOnHold      CallState = "ON_HOLD"
	StateTransferring CallState = "TRANSFERRING"
)

// CallSession represents a complete call session
type CallSession struct {
	ID                string                 `json:"id"`
	State             CallState              `json:"state"`
	CallerID          string                 `json:"caller_id"`
	CallerNumber      string                 `json:"caller_number"`
	CalledNumber      string                 `json:"called_number"`
	Direction         string                 `json:"direction"` // inbound, outbound
	StartTime         time.Time              `json:"start_time"`
	EndTime           *time.Time             `json:"end_time,omitempty"`
	Duration          int                    `json:"duration"` // seconds
	LLMTurnCount      int                    `json:"llm_turn_count"`
	LastActivity      time.Time              `json:"last_activity"`
	Transcript        []TranscriptEntry      `json:"transcript"`
	LLMSummary        string                 `json:"llm_summary"`
	TransferReason    string                 `json:"transfer_reason"`
	Intent            string                 `json:"intent"`
	Confidence        float64                `json:"confidence"`
	AgentID           string                 `json:"agent_id"`
	AgentName         string                 `json:"agent_name"`
	MediaSessionID    string                 `json:"media_session_id"`
	SIPCallID         string                 `json:"sip_call_id"`
	Metadata          map[string]interface{} `json:"metadata"`
	mutex             sync.RWMutex           `json:"-"`
}

// TranscriptEntry represents a single entry in the call transcript
type TranscriptEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Speaker   string    `json:"speaker"` // caller, llm, agent
	Text      string    `json:"text"`
	AudioURL  string    `json:"audio_url,omitempty"`
}

// CallManager manages call sessions
type CallManager struct {
	redis    *redis.Client
	sessions map[string]*CallSession
	mutex    sync.RWMutex
	ttl      time.Duration
}

// NewCallManager creates a new call manager
func NewCallManager(redisClient *redis.Client) *CallManager {
	return &CallManager{
		redis:    redisClient,
		sessions: make(map[string]*CallSession),
		ttl:      24 * time.Hour,
	}
}

// CreateSession creates a new call session
func (cm *CallManager) CreateSession(ctx context.Context, callerID, callerNumber, calledNumber string) (*CallSession, error) {
	session := &CallSession{
		ID:           uuid.New().String(),
		State:        StateIncoming,
		CallerID:     callerID,
		CallerNumber: callerNumber,
		CalledNumber: calledNumber,
		Direction:    "inbound",
		StartTime:    time.Now(),
		LastActivity: time.Now(),
		Transcript:   make([]TranscriptEntry, 0),
		Metadata:     make(map[string]interface{}),
	}

	// Store in memory
	cm.mutex.Lock()
	cm.sessions[session.ID] = session
	cm.mutex.Unlock()

	// Store in Redis
	if err := cm.persistSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to persist session: %w", err)
	}

	return session, nil
}

// GetSession retrieves a call session by ID
func (cm *CallManager) GetSession(ctx context.Context, sessionID string) (*CallSession, error) {
	// Try memory first
	cm.mutex.RLock()
	session, exists := cm.sessions[sessionID]
	cm.mutex.RUnlock()

	if exists {
		return session, nil
	}

	// Try Redis
	session, err := cm.loadSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Cache in memory
	cm.mutex.Lock()
	cm.sessions[sessionID] = session
	cm.mutex.Unlock()

	return session, nil
}

// UpdateState updates the call state
func (cm *CallManager) UpdateState(ctx context.Context, sessionID string, newState CallState) error {
	session, err := cm.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.mutex.Lock()
	oldState := session.State
	
	// Validate state transition
	if !oldState.CanTransitionTo(newState) {
		session.mutex.Unlock()
		return fmt.Errorf("invalid state transition: %s -> %s", oldState, newState)
	}
	
	session.State = newState
	session.LastActivity = time.Now()

	if newState == StateTerminated {
		now := time.Now()
		session.EndTime = &now
		session.Duration = int(now.Sub(session.StartTime).Seconds())
	}
	session.mutex.Unlock()

	// Persist to Redis
	if err := cm.persistSession(ctx, session); err != nil {
		return fmt.Errorf("failed to persist state change: %w", err)
	}

	// Publish state change event
	if cm.redis != nil {
		event := map[string]interface{}{
			"type":       "state_change",
			"session_id": sessionID,
			"old_state":  oldState,
			"new_state":  newState,
			"timestamp":  time.Now().Unix(),
		}
		eventJSON, _ := json.Marshal(event)
		cm.redis.Publish(ctx, "call_events", eventJSON)
	}

	return nil
}

// AddTranscriptEntry adds a transcript entry
func (cm *CallManager) AddTranscriptEntry(ctx context.Context, sessionID string, entry TranscriptEntry) error {
	session, err := cm.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.mutex.Lock()
	session.Transcript = append(session.Transcript, entry)
	session.LastActivity = time.Now()
	session.mutex.Unlock()

	// Update LLM turn count if speaker is LLM
	if entry.Speaker == "llm" {
		session.LLMTurnCount++
	}

	return cm.persistSession(ctx, session)
}

// UpdateLLMResponse updates LLM response data
func (cm *CallManager) UpdateLLMResponse(ctx context.Context, sessionID string, response string, confidence float64, intent string) error {
	session, err := cm.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.mutex.Lock()
	session.LLMSummary = response
	session.Confidence = confidence
	session.Intent = intent
	session.LastActivity = time.Now()
	session.mutex.Unlock()

	return cm.persistSession(ctx, session)
}

// SetTransferData sets transfer-related data
func (cm *CallManager) SetTransferData(ctx context.Context, sessionID string, agentID, agentName, reason string) error {
	session, err := cm.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.mutex.Lock()
	session.AgentID = agentID
	session.AgentName = agentName
	session.TransferReason = reason
	session.LastActivity = time.Now()
	session.mutex.Unlock()

	return cm.persistSession(ctx, session)
}

// GetActiveSessions returns all active sessions
func (cm *CallManager) GetActiveSessions() []*CallSession {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var active []*CallSession
	for _, session := range cm.sessions {
		session.mutex.RLock()
		if session.State != StateTerminated {
			active = append(active, session)
		}
		session.mutex.RUnlock()
	}

	return active
}

// GetSessionsByState returns sessions filtered by state
func (cm *CallManager) GetSessionsByState(state CallState) []*CallSession {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var filtered []*CallSession
	for _, session := range cm.sessions {
		session.mutex.RLock()
		if session.State == state {
			filtered = append(filtered, session)
		}
		session.mutex.RUnlock()
	}

	return filtered
}

// GetAgentSessions returns sessions assigned to an agent
func (cm *CallManager) GetAgentSessions(agentID string) []*CallSession {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var assigned []*CallSession
	for _, session := range cm.sessions {
		session.mutex.RLock()
		if session.AgentID == agentID && session.State != StateTerminated {
			assigned = append(assigned, session)
		}
		session.mutex.RUnlock()
	}

	return assigned
}

// CloseSession terminates a session
func (cm *CallManager) CloseSession(ctx context.Context, sessionID string) error {
	session, err := cm.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.mutex.Lock()
	now := time.Now()
	session.EndTime = &now
	session.Duration = int(now.Sub(session.StartTime).Seconds())
	session.State = StateTerminated
	session.mutex.Unlock()

	// Persist final state
	if err := cm.persistSession(ctx, session); err != nil {
		return err
	}

	// Remove from memory after some delay (for cleanup)
	go func() {
		time.Sleep(5 * time.Minute)
		cm.mutex.Lock()
		delete(cm.sessions, sessionID)
		cm.mutex.Unlock()
	}()

	return nil
}

// Cleanup removes terminated sessions from memory
func (cm *CallManager) Cleanup() {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for id, session := range cm.sessions {
		session.mutex.RLock()
		if session.State == StateTerminated {
			delete(cm.sessions, id)
		}
		session.mutex.RUnlock()
	}
}

// persistSession saves session to Redis
func (cm *CallManager) persistSession(ctx context.Context, session *CallSession) error {
	if cm.redis == nil {
		return nil
	}

	session.mutex.RLock()
	data, err := json.Marshal(session)
	session.mutex.RUnlock()

	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	key := fmt.Sprintf("call_session:%s", session.ID)
	if err := cm.redis.Set(ctx, key, data, cm.ttl).Err(); err != nil {
		return fmt.Errorf("failed to save to Redis: %w", err)
	}

	return nil
}

// loadSession retrieves session from Redis
func (cm *CallManager) loadSession(ctx context.Context, sessionID string) (*CallSession, error) {
	if cm.redis == nil {
		return nil, fmt.Errorf("Redis not available")
	}

	key := fmt.Sprintf("call_session:%s", sessionID)
	data, err := cm.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get from Redis: %w", err)
	}

	var session CallSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// State transitions
func (s CallState) CanTransitionTo(newState CallState) bool {
	transitions := map[CallState][]CallState{
		StateIncoming:     {StateLLMRouting, StateTerminated, StateLiveAgent, StateTransferring},
		StateLLMRouting:  {StateLiveAgent, StateTerminated, StateTransferring, StateOnHold},
		StateLiveAgent:   {StateOnHold, StateTerminated, StateTransferring},
		StateOnHold:      {StateLiveAgent, StateLLMRouting, StateTerminated},
		StateTransferring: {StateLiveAgent, StateTerminated},
		StateTerminated:  {},
	}

	validStates, exists := transitions[s]
	if !exists {
		return false
	}

	for _, valid := range validStates {
		if valid == newState {
			return true
		}
	}

	return false
}
