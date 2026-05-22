package types

import (
	"sync"
	"time"
)

// CallState represents the current state of a call
type CallState string

const (
	StateIncoming     CallState = "INCOMING"
	StateLLMRouting   CallState = "LLM_ROUTING"
	StateLiveAgent    CallState = "LIVE_AGENT"
	StateTerminated   CallState = "TERMINATED"
	StateOnHold       CallState = "ON_HOLD"
	StateTransferring CallState = "TRANSFERRING"
)

// CallSession represents a complete call session
type CallSession struct {
	ID             string                 `json:"id"`
	State          CallState              `json:"state"`
	CallerID       string                 `json:"caller_id"`
	CallerNumber   string                 `json:"caller_number"`
	CalledNumber   string                 `json:"called_number"`
	Direction      string                 `json:"direction"`
	StartTime      time.Time              `json:"start_time"`
	EndTime        *time.Time             `json:"end_time,omitempty"`
	Duration       int                    `json:"duration"`
	LLMTurnCount   int                    `json:"llm_turn_count"`
	LastActivity   time.Time              `json:"last_activity"`
	Transcript     []TranscriptEntry      `json:"transcript"`
	LLMSummary     string                 `json:"llm_summary"`
	TransferReason string                 `json:"transfer_reason"`
	Intent         string                 `json:"intent"`
	Confidence     float64                `json:"confidence"`
	AgentID        string                 `json:"agent_id"`
	AgentName      string                 `json:"agent_name"`
	MediaSessionID string                 `json:"media_session_id"`
	SIPCallID      string                 `json:"sip_call_id"`
	Metadata       map[string]interface{} `json:"metadata"`
	// Mutex is embedded for caller synchronization.  Not serialized.
	Mutex          sync.RWMutex           `json:"-"`
}

// TranscriptEntry represents a single entry in the call transcript
type TranscriptEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Speaker   string    `json:"speaker"`
	Text      string    `json:"text"`
	AudioURL  string    `json:"audio_url,omitempty"`
}

// State transitions
func (s CallState) CanTransitionTo(newState CallState) bool {
	transitions := map[CallState][]CallState{
		StateIncoming:     {StateLLMRouting, StateTerminated, StateLiveAgent, StateTransferring},
		StateLLMRouting:   {StateLiveAgent, StateTerminated, StateTransferring, StateOnHold},
		StateLiveAgent:    {StateOnHold, StateTerminated, StateTransferring},
		StateOnHold:       {StateLiveAgent, StateLLMRouting, StateTerminated},
		StateTransferring: {StateLiveAgent, StateTerminated},
		StateTerminated:   {},
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

// CanTransitionTo returns whether a transition is valid (convenience wrapper)
func CanTransitionTo(s, newState CallState) bool {
	return s.CanTransitionTo(newState)
}
