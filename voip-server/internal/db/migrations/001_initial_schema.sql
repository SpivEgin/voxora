-- CockroachDB Schema Migration: Initial Tables
-- License: GNU GPL v3.0

-- ============================================================================
-- Call Sessions Table — Primary durable store for all call records
-- ============================================================================

CREATE TABLE IF NOT EXISTS call_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    state VARCHAR(32) NOT NULL,
    caller_id VARCHAR(64) NOT NULL,
    caller_number VARCHAR(32),
    called_number VARCHAR(32),
    direction VARCHAR(16) DEFAULT 'inbound',
    start_time TIMESTAMPTZ DEFAULT now(),
    end_time TIMESTAMPTZ,
    duration_seconds INT DEFAULT 0,
    llm_turn_count INT DEFAULT 0,
    intent VARCHAR(64),
    confidence FLOAT,
    agent_id VARCHAR(64),
    agent_name VARCHAR(128),
    transfer_reason VARCHAR(256),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- Indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_calls_state ON call_sessions(state);
CREATE INDEX IF NOT EXISTS idx_calls_caller ON call_sessions(caller_id);
CREATE INDEX IF NOT EXISTS idx_calls_time ON call_sessions(start_time DESC);
CREATE INDEX IF NOT EXISTS idx_calls_agent ON call_sessions(agent_id) WHERE agent_id IS NOT NULL;

-- ============================================================================
-- Transcripts Table — Conversation history with foreign key to sessions
-- ============================================================================

CREATE TABLE IF NOT EXISTS transcripts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES call_sessions(id) ON DELETE CASCADE,
    speaker VARCHAR(32) NOT NULL,
    text TEXT NOT NULL,
    audio_url VARCHAR(512),
    timestamp TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_transcripts_session ON transcripts(session_id, timestamp DESC);

-- ============================================================================
-- Agents Table (optional) — Track available human agents
-- ============================================================================

CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(128),
    status VARCHAR(32) DEFAULT 'offline',  -- offline, available, busy, paused
    queue_name VARCHAR(64) DEFAULT 'default',
    max_concurrent_calls INT DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);

-- ============================================================================
-- Call Events Table — Audit log for all state changes
-- ============================================================================

CREATE TABLE IF NOT EXISTS call_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES call_sessions(id) ON DELETE CASCADE,
    event_type VARCHAR(64) NOT NULL,
    old_state VARCHAR(32),
    new_state VARCHAR(32),
    details JSONB,
    agent_id VARCHAR(64),
    ip_address VARCHAR(64),
    timestamp TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_events_session ON call_events(session_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_events_type ON call_events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_time ON call_events(timestamp DESC);

-- ============================================================================
-- Statistics Table (optional) — Aggregated call metrics for reporting
-- ============================================================================

CREATE TABLE IF NOT EXISTS call_stats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES call_sessions(id) ON DELETE CASCADE,
    total_duration_seconds INT DEFAULT 0,
    llm_turns INT DEFAULT 0,
    transferred BOOLEAN DEFAULT FALSE,
    transfer_target VARCHAR(64),
    final_state VARCHAR(32),
    caller_satisfaction INT, -- 1-5 scale, nullable
    tags VARCHAR(256)[] DEFAULT ARRAY[]::VARCHAR[],
    created_at TIMESTAMPTZ DEFAULT now()
);

-- ============================================================================
-- Comments for documentation
-- ============================================================================

COMMENT ON TABLE call_sessions IS 'Primary table storing all VoIP call sessions';
COMMENT ON TABLE transcripts IS 'Conversation entries linked to call_sessions';
COMMENT ON TABLE agents IS 'Human agent registry for transfer routing';
COMMENT ON TABLE call_events IS 'Immutable audit log of call lifecycle events';
COMMENT ON TABLE call_stats IS 'Aggregated statistics for analytics/reporting';
