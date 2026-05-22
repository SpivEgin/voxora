package state

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallManager_CreateSession(t *testing.T) {
	cm := NewCallManager(nil)
	ctx := context.Background()

	session, err := cm.CreateSession(ctx, "caller123", "+1234567890", "+0987654321")
	require.NoError(t, err)
	require.NotNil(t, session)

	assert.NotEmpty(t, session.ID)
	assert.Equal(t, StateIncoming, session.State)
	assert.Equal(t, "caller123", session.CallerID)
	assert.Equal(t, "+1234567890", session.CallerNumber)
	assert.Equal(t, "+0987654321", session.CalledNumber)
	assert.Equal(t, "inbound", session.Direction)
	assert.WithinDuration(t, time.Now(), session.StartTime, time.Second)
	assert.Empty(t, session.Transcript)
}

func TestCallManager_GetSession(t *testing.T) {
	cm := NewCallManager(nil)
	ctx := context.Background()

	// Create session
	session, err := cm.CreateSession(ctx, "caller123", "+1234567890", "+0987654321")
	require.NoError(t, err)

	// Get session
	retrieved, err := cm.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ID, retrieved.ID)
	assert.Equal(t, session.CallerID, retrieved.CallerID)

	// Get non-existent session
	_, err = cm.GetSession(ctx, "non-existent")
	assert.Error(t, err)
}

func TestCallManager_UpdateState(t *testing.T) {
	cm := NewCallManager(nil)
	ctx := context.Background()

	session, err := cm.CreateSession(ctx, "caller123", "+1234567890", "+0987654321")
	require.NoError(t, err)

	// Test valid state transitions
	tests := []struct {
		from CallState
		to   CallState
		ok   bool
	}{
		{StateIncoming, StateLLMRouting, true},
		{StateLLMRouting, StateLiveAgent, true},
		{StateLiveAgent, StateTerminated, true},
		{StateTerminated, StateIncoming, false},
	}

	for _, tt := range tests {
		err := cm.UpdateState(ctx, session.ID, tt.to)
		if tt.ok {
			assert.NoError(t, err)
			updated, _ := cm.GetSession(ctx, session.ID)
			assert.Equal(t, tt.to, updated.State)
		} else {
			assert.Error(t, err)
		}
	}
}

func TestCallManager_AddTranscriptEntry(t *testing.T) {
	cm := NewCallManager(nil)
	ctx := context.Background()

	session, err := cm.CreateSession(ctx, "caller123", "+1234567890", "+0987654321")
	require.NoError(t, err)

	entry := TranscriptEntry{
		Timestamp: time.Now(),
		Speaker:   "caller",
		Text:      "Hello, I need help",
	}

	err = cm.AddTranscriptEntry(ctx, session.ID, entry)
	require.NoError(t, err)

	updated, err := cm.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, updated.Transcript, 1)
	assert.Equal(t, "caller", updated.Transcript[0].Speaker)
	assert.Equal(t, "Hello, I need help", updated.Transcript[0].Text)
}

func TestCallManager_GetActiveSessions(t *testing.T) {
	cm := NewCallManager(nil)
	ctx := context.Background()

	// Create active session
	session1, _ := cm.CreateSession(ctx, "caller1", "+1", "+2")
	_ = cm.UpdateState(ctx, session1.ID, StateLLMRouting)

	// Create and terminate session
	session2, _ := cm.CreateSession(ctx, "caller2", "+3", "+4")
	_ = cm.UpdateState(ctx, session2.ID, StateTerminated)

	active := cm.GetActiveSessions()
	assert.Len(t, active, 1)
	assert.Equal(t, session1.ID, active[0].ID)
}

func TestCallManager_GetSessionsByState(t *testing.T) {
	cm := NewCallManager(nil)
	ctx := context.Background()

	// Create sessions
	session1, _ := cm.CreateSession(ctx, "caller1", "+1", "+2")
	_ = cm.UpdateState(ctx, session1.ID, StateLLMRouting)

	session2, _ := cm.CreateSession(ctx, "caller2", "+3", "+4")
	_ = cm.UpdateState(ctx, session2.ID, StateLLMRouting)

	session3, _ := cm.CreateSession(ctx, "caller3", "+5", "+6")
	_ = cm.UpdateState(ctx, session3.ID, StateLiveAgent)

	llmSessions := cm.GetSessionsByState(StateLLMRouting)
	assert.Len(t, llmSessions, 2)

	agentSessions := cm.GetSessionsByState(StateLiveAgent)
	assert.Len(t, agentSessions, 1)
}

func TestCallManager_CloseSession(t *testing.T) {
	cm := NewCallManager(nil)
	ctx := context.Background()

	session, err := cm.CreateSession(ctx, "caller123", "+1234567890", "+0987654321")
	require.NoError(t, err)

	err = cm.CloseSession(ctx, session.ID)
	require.NoError(t, err)

	// Verify session is terminated
	updated, err := cm.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, StateTerminated, updated.State)
	assert.NotNil(t, updated.EndTime)
}

func TestCallState_CanTransitionTo(t *testing.T) {
	tests := []struct {
		state    CallState
		target   CallState
		expected bool
	}{
		{StateIncoming, StateLLMRouting, true},
		{StateIncoming, StateTerminated, true},
		{StateIncoming, StateLiveAgent, true},
		{StateIncoming, StateOnHold, false},
		{StateLLMRouting, StateLiveAgent, true},
		{StateLLMRouting, StateTerminated, true},
		{StateLLMRouting, StateTransferring, true},
		{StateLLMRouting, StateIncoming, false},
		{StateLiveAgent, StateOnHold, true},
		{StateLiveAgent, StateTerminated, true},
		{StateTerminated, StateIncoming, false},
		{StateTerminated, StateLLMRouting, false},
	}

	for _, tt := range tests {
		result := tt.state.CanTransitionTo(tt.target)
		assert.Equal(t, tt.expected, result, 
			"Expected %s->%s to be %v", tt.state, tt.target, tt.expected)
	}
}
