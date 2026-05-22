package transfer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"voip-server/internal/config"
	"voip-server/internal/llm"
	"voip-server/internal/state"
	"voip-server/internal/types"
)

func TestEngine_Evaluate(t *testing.T) {
	cm := state.NewCallManager(nil, nil)
	cfg := &config.TransferConfig{
		Enabled: true,
		Conditions: config.TransferConditions{
			CallerRequestsHuman: true,
			ConfidenceThreshold: 0.6,
			IntentTriggers:      []string{"complaint", "emergency", "refund"},
			MaxLLMTurns:         10,
			MaxCallDuration:     600,
			AbuseDetection:      true,
		},
		Targets: []config.TransferTarget{
			{Name: "support", SIPURI: "sip:support@queue.local"},
			{Name: "billing", SIPURI: "sip:billing@queue.local"},
		},
	}
	
	engine := NewEngine(cfg, cm, nil)
	ctx := context.Background()

	tests := []struct {
		name       string
		transcript []string
		llmResult  *llm.CompletionResult
		expected   bool
		reason     string
	}{
		{
			name:       "caller requests human",
			transcript: []string{"I want to speak to a human"},
			llmResult:  &llm.CompletionResult{Confidence: 0.9, Intent: "general"},
			expected:   true,
			reason:     "caller_requested_human",
		},
		{
			name:       "low confidence",
			transcript: []string{"Hello"},
			llmResult:  &llm.CompletionResult{Confidence: 0.4, Intent: "general"},
			expected:   true,
			reason:     "low_confidence",
		},
		{
			name:       "complaint intent",
			transcript: []string{"I have a complaint"},
			llmResult:  &llm.CompletionResult{Confidence: 0.9, Intent: "complaint"},
			expected:   true,
			reason:     "intent_trigger",
		},
		{
			name:       "normal inquiry",
			transcript: []string{"What are your hours?"},
			llmResult:  &llm.CompletionResult{Confidence: 0.9, Intent: "general"},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, _ := cm.CreateSession(ctx, "test", "+1", "+2")
			defer cm.CloseSession(ctx, session.ID)

			result, err := engine.Evaluate(ctx, session.ID, tt.llmResult, tt.transcript)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.ShouldTransfer)
			
			if tt.expected {
				assert.Contains(t, result.Reason, tt.reason)
			}
		})
	}
}

func TestEngine_checkCallerRequestsHuman(t *testing.T) {
	cfg := &config.TransferConfig{
		Enabled: true,
		Conditions: config.TransferConditions{
			CallerRequestsHuman: true,
		},
	}
	engine := NewEngine(cfg, nil, nil)

	tests := []struct {
		transcript []string
		expected   bool
	}{
		{[]string{"I need a human"}, true},
		{[]string{"Connect me to an agent"}, true},
		{[]string{"Can I speak to a person?"}, true},
		{[]string{"Transfer me to a representative"}, true},
		{[]string{"What time do you close?"}, false},
		{[]string{"I need help with my account"}, false},
	}

	for _, tt := range tests {
		result := engine.checkCallerRequestsHuman(tt.transcript)
		assert.Equal(t, tt.expected, result)
	}
}

func TestEngine_findBestTarget(t *testing.T) {
	cfg := &config.TransferConfig{
		Targets: []config.TransferTarget{
			{Name: "sales", SIPURI: "sip:sales@queue.local"},
			{Name: "support", SIPURI: "sip:support@queue.local"},
			{Name: "billing", SIPURI: "sip:billing@queue.local"},
			{Name: "emergency", SIPURI: "sip:emergency@queue.local"},
		},
	}
	engine := NewEngine(cfg, nil, nil)

	tests := []struct {
		intent   string
		expected string
	}{
		{"sales", "sales"},
		{"buy", "sales"},
		{"technical", "support"},
		{"support", "support"},
		{"billing", "billing"},
		{"payment", "billing"},
		{"emergency", "emergency"},
		{"complaint", "support"}, // Default
		{"unknown", "support"},   // Default target
	}

	for _, tt := range tests {
		target := engine.findBestTarget(tt.intent)
		require.NotNil(t, target)
		assert.Equal(t, tt.expected, target.Name)
	}
}

func TestEngine_RequestTransfer(t *testing.T) {
	cm := state.NewCallManager(nil, nil)
	cfg := &config.TransferConfig{
		Enabled: true,
		Targets: []config.TransferTarget{
			{Name: "support", SIPURI: "sip:support@queue.local"},
		},
	}
	engine := NewEngine(cfg, cm, nil)
	ctx := context.Background()

	session, _ := cm.CreateSession(ctx, "test", "+1", "+2")

	ctx2, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	
	engine.Start(ctx2)
	defer engine.Stop()

	target := &cfg.Targets[0]
	err := engine.RequestTransfer(ctx, session.ID, target, "test_reason")
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Verify state changed to LIVE_AGENT (transfers immediately transition)
	updated, _ := cm.GetSession(ctx, session.ID)
	assert.Equal(t, types.StateLiveAgent, updated.State)
}
