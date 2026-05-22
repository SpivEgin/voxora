package state

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"voip-server/internal/db"
	"voip-server/internal/types"
)

// CallManager manages call sessions using CockroachDB as the primary
// durable store and Redis as an optional hot cache / pub-sub layer.
type CallManager struct {
	crdb     *db.CockroachDB
	redis    *redis.Client
	sessions map[string]*types.CallSession
	mutex    sync.RWMutex
	ttl      time.Duration
	logger   *zap.Logger
}

// NewCallManager creates a new call manager. crdb is the primary durable
// store; redisClient is optional (can be nil) and used as a hot cache
// and/or pub-sub. If crdb is nil, the manager falls back to in-memory
// storage with Redis if available.
func NewCallManager(crdb *db.CockroachDB, redisClient *redis.Client) *CallManager {
	return &CallManager{
		crdb:     crdb,
		redis:    redisClient,
		sessions: make(map[string]*types.CallSession),
		ttl:      24 * time.Hour,
	}
}

// SetLogger injects a logger after construction (avoids requiring it in
// NewCallManager so that callers in tests don't have to provide one).
func (cm *CallManager) SetLogger(l *zap.Logger) {
	cm.logger = l
}

// CreateSession creates a new call session and immediately writes it to
// CockroachDB. Redis (if configured) is updated asynchronously.
func (cm *CallManager) CreateSession(ctx context.Context, callerID, callerNumber, calledNumber string) (*types.CallSession, error) {
	session := &types.CallSession{
		ID:           uuid.New().String(),
		State:        types.StateIncoming,
		CallerID:     callerID,
		CallerNumber: callerNumber,
		CalledNumber: calledNumber,
		Direction:    "inbound",
		StartTime:    time.Now(),
		LastActivity: time.Now(),
		Transcript:   make([]types.TranscriptEntry, 0),
		Metadata:     make(map[string]interface{}),
	}

	// Store in memory
	cm.mutex.Lock()
	cm.sessions[session.ID] = session
	cm.mutex.Unlock()

	// Write to CockroachDB (primary durable store)
	if cm.crdb != nil {
		if err := cm.crdb.SaveSession(ctx, session); err != nil {
			if cm.logger != nil {
				cm.logger.Warn("Failed to persist session to CockroachDB",
					zap.String("session_id", session.ID),
					zap.Error(err))
			}
		}
	}

	// Update Redis cache asynchronously
	go cm.persistToRedis(session)

	return session, nil
}

// GetSession retrieves a call session by ID. Memory is checked first,
// then CockroachDB (if available), then Redis as a fallback.
func (cm *CallManager) GetSession(ctx context.Context, sessionID string) (*types.CallSession, error) {
	// Try memory first
	cm.mutex.RLock()
	session, exists := cm.sessions[sessionID]
	cm.mutex.RUnlock()
	if exists {
		return session, nil
	}

	// Try CockroachDB (primary durable store)
	if cm.crdb != nil {
		session, err := cm.crdb.GetSession(ctx, sessionID)
		if err == nil {
			// Cache in memory
			cm.mutex.Lock()
			cm.sessions[sessionID] = session
			cm.mutex.Unlock()
			return session, nil
		}
		if cm.logger != nil {
			cm.logger.Debug("CockroachDB session miss",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}

	// Try Redis as fallback
	session, err := cm.loadFromRedis(ctx, sessionID)
	if err == nil {
		cm.mutex.Lock()
		cm.sessions[sessionID] = session
		cm.mutex.Unlock()
		return session, nil
	}

	return nil, fmt.Errorf("session not found: %s", sessionID)
}

// UpdateState updates the call state and writes the change to CockroachDB.
func (cm *CallManager) UpdateState(ctx context.Context, sessionID string, newState types.CallState) error {
	session, err := cm.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Mutex.Lock()
	oldState := session.State
	if !oldState.CanTransitionTo(newState) {
		session.Mutex.Unlock()
		return fmt.Errorf("invalid state transition: %s -> %s", oldState, newState)
	}
	session.State = newState
	session.LastActivity = time.Now()
	if newState == types.StateTerminated {
		now := time.Now()
		session.EndTime = &now
		session.Duration = int(now.Sub(session.StartTime).Seconds())
	}
	session.Mutex.Unlock()

	// Update CockroachDB (primary)
	if cm.crdb != nil {
		if err := cm.crdb.SaveSession(ctx, session); err != nil {
			if cm.logger != nil {
				cm.logger.Warn("Failed to update state in CockroachDB",
					zap.String("session_id", sessionID), zap.Error(err))
			}
		}
	}

	// Update Redis cache
	go cm.persistToRedis(session)

	// Publish state change event
	if cm.redis != nil {
		publishStateChange(cm.redis, sessionID, oldState, newState)
	}

	return nil
}

// AddTranscriptEntry adds a transcript entry to memory, CockroachDB, and Redis.
func (cm *CallManager) AddTranscriptEntry(ctx context.Context, sessionID string, entry types.TranscriptEntry) error {
	session, err := cm.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Mutex.Lock()
	session.Transcript = append(session.Transcript, entry)
	session.LastActivity = time.Now()
	if entry.Speaker == "llm" {
		session.LLMTurnCount++
	}
	session.Mutex.Unlock()

	// Write to CockroachDB (primary)
	if cm.crdb != nil {
		if err := cm.crdb.AddTranscriptEntry(ctx, sessionID, entry); err != nil {
			if cm.logger != nil {
				cm.logger.Warn("Failed to write transcript to CockroachDB",
					zap.String("session_id", sessionID), zap.Error(err))
			}
		}
	}

	// Update Redis cache
	go cm.persistToRedis(session)

	return nil
}

// UpdateLLMResponse updates LLM response data in memory and persists to CockroachDB.
func (cm *CallManager) UpdateLLMResponse(ctx context.Context, sessionID string, response string, confidence float64, intent string) error {
	session, err := cm.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Mutex.Lock()
	session.LLMSummary = response
	session.Confidence = confidence
	session.Intent = intent
	session.LastActivity = time.Now()
	session.Mutex.Unlock()

	if cm.crdb != nil {
		if err := cm.crdb.SaveSession(ctx, session); err != nil {
			if cm.logger != nil {
				cm.logger.Warn("Failed to update LLM response in CockroachDB",
					zap.String("session_id", sessionID), zap.Error(err))
			}
		}
	}

	go cm.persistToRedis(session)

	return nil
}

// SetTransferData sets transfer-related data and persists to CockroachDB.
func (cm *CallManager) SetTransferData(ctx context.Context, sessionID string, agentID, agentName, reason string) error {
	session, err := cm.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Mutex.Lock()
	session.AgentID = agentID
	session.AgentName = agentName
	session.TransferReason = reason
	session.LastActivity = time.Now()
	session.Mutex.Unlock()

	if cm.crdb != nil {
		if err := cm.crdb.SaveSession(ctx, session); err != nil {
			if cm.logger != nil {
				cm.logger.Warn("Failed to update transfer data in CockroachDB",
					zap.String("session_id", sessionID), zap.Error(err))
			}
		}
	}

	go cm.persistToRedis(session)

	return nil
}

// CloseSession terminates a session and records the final state in CockroachDB.
func (cm *CallManager) CloseSession(ctx context.Context, sessionID string) error {
	session, err := cm.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Mutex.Lock()
	now := time.Now()
	session.EndTime = &now
	session.Duration = int(now.Sub(session.StartTime).Seconds())
	session.State = types.StateTerminated
	session.Mutex.Unlock()

	if cm.crdb != nil {
		if err := cm.crdb.SaveSession(ctx, session); err != nil {
			if cm.logger != nil {
				cm.logger.Warn("Failed to close session in CockroachDB",
					zap.String("session_id", sessionID), zap.Error(err))
			}
		}
	}

	go cm.persistToRedis(session)

	// Remove from memory after delay
	go func() {
		time.Sleep(5 * time.Minute)
		cm.mutex.Lock()
		delete(cm.sessions, sessionID)
		cm.mutex.Unlock()
	}()

	return nil
}

// GetActiveSessions returns all non-terminated sessions from memory.
func (cm *CallManager) GetActiveSessions() []*types.CallSession {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var active []*types.CallSession
	for _, session := range cm.sessions {
		session.Mutex.RLock()
		if session.State != types.StateTerminated {
			active = append(active, session)
		}
		session.Mutex.RUnlock()
	}
	return active
}

// GetSessionsByState returns sessions filtered by state from memory.
func (cm *CallManager) GetSessionsByState(state types.CallState) []*types.CallSession {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var filtered []*types.CallSession
	for _, session := range cm.sessions {
		session.Mutex.RLock()
		if session.State == state {
			filtered = append(filtered, session)
		}
		session.Mutex.RUnlock()
	}
	return filtered
}

// GetAgentSessions returns sessions assigned to an agent from memory.
func (cm *CallManager) GetAgentSessions(agentID string) []*types.CallSession {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var assigned []*types.CallSession
	for _, session := range cm.sessions {
		session.Mutex.RLock()
		if session.AgentID == agentID && session.State != types.StateTerminated {
			assigned = append(assigned, session)
		}
		session.Mutex.RUnlock()
	}
	return assigned
}

// Cleanup removes terminated sessions from memory.
func (cm *CallManager) Cleanup() {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for id, session := range cm.sessions {
		session.Mutex.RLock()
		if session.State == types.StateTerminated {
			delete(cm.sessions, id)
		}
		session.Mutex.RUnlock()
	}
}

// persistToRedis stores a session in Redis asynchronously.
func (cm *CallManager) persistToRedis(session *types.CallSession) {
	if cm.redis == nil {
		return
	}
	session.Mutex.RLock()
	data, err := json.Marshal(session)
	session.Mutex.RUnlock()
	if err != nil {
		return
	}
	key := fmt.Sprintf("call_session:%s", session.ID)
	_ = cm.redis.Set(context.Background(), key, data, cm.ttl).Err()
}

// loadFromRedis retrieves a session from Redis.
func (cm *CallManager) loadFromRedis(ctx context.Context, sessionID string) (*types.CallSession, error) {
	if cm.redis == nil {
		return nil, fmt.Errorf("redis not available")
	}
	key := fmt.Sprintf("call_session:%s", sessionID)
	data, err := cm.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var session types.CallSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// publishStateChange publishes a state change event via Redis.
func publishStateChange(redisClient *redis.Client, sessionID string, oldState, newState types.CallState) {
	event := map[string]interface{}{
		"type":       "state_change",
		"session_id": sessionID,
		"old_state":  oldState,
		"new_state":  newState,
		"timestamp":  time.Now().Unix(),
	}
	eventJSON, _ := json.Marshal(event)
	_ = redisClient.Publish(context.Background(), "call_events", eventJSON).Err()
}
