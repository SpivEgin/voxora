package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"voip-server/internal/config"
	"voip-server/internal/types"
)

// CockroachDB implements the primary durable datastore for call sessions
type CockroachDB struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
	config *config.CockroachDBConfig
}

// New creates a new CockroachDB connection pool
func New(cfg *config.CockroachDBConfig, logger *zap.Logger) (*CockroachDB, error) {
	if !cfg.Enabled {
		logger.Info("CockroachDB disabled, using in-memory store only")
		return nil, nil
	}

	connStr := buildConnString(cfg)

	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CockroachDB config: %w", err)
	}

	// Connection pool settings
	config.MaxConns = int32(cfg.MaxOpenConns)
	config.MinConns = int32(cfg.MaxIdleConns)
	config.MaxConnLifetime = time.Duration(cfg.ConnMaxLifetime) * time.Second
	config.MaxConnIdleTime = time.Duration(cfg.ConnMaxIdleTime) * time.Second

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create CockroachDB pool: %w", err)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping CockroachDB: %w", err)
	}

	db := &CockroachDB{
		pool:   pool,
		logger: logger,
		config: cfg,
	}

	// Run migrations if enabled
	if cfg.MigrationsEnabled {
		if err := db.runMigrations(ctx); err != nil {
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	}

	logger.Info("CockroachDB connected",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Database))

	return db, nil
}

// Close closes the connection pool
func (db *CockroachDB) Close() {
	if db.pool != nil {
		db.pool.Close()
	}
}

// SaveSession persists a call session to CockroachDB
func (db *CockroachDB) SaveSession(ctx context.Context, session *types.CallSession) error {
	if db.pool == nil {
		return nil // DB disabled
	}

	sql := `
		UPSERT INTO call_sessions 
		(id, state, caller_id, caller_number, called_number, direction, 
		 start_time, end_time, duration_seconds, llm_turn_count, 
		 intent, confidence, agent_id, agent_name, transfer_reason, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, now())
	`

	_, err := db.pool.Exec(ctx, sql,
		session.ID,
		string(session.State),
		session.CallerID,
		session.CallerNumber,
		session.CalledNumber,
		session.Direction,
		session.StartTime,
		session.EndTime,
		session.Duration,
		session.LLMTurnCount,
		session.Intent,
		session.Confidence,
		session.AgentID,
		session.AgentName,
		session.TransferReason,
	)

	if err != nil {
		db.logger.Error("Failed to save session to CockroachDB", zap.Error(err))
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// GetSession retrieves a call session by ID
func (db *CockroachDB) GetSession(ctx context.Context, id string) (*types.CallSession, error) {
	if db.pool == nil {
		return nil, fmt.Errorf("database not enabled")
	}

	sql := `
		SELECT id, state, caller_id, caller_number, called_number, direction,
		       start_time, end_time, duration_seconds, llm_turn_count,
		       intent, confidence, agent_id, agent_name, transfer_reason, created_at
		FROM call_sessions WHERE id = $1
	`

	row, err := db.pool.Query(ctx, sql, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}
	defer row.Close()

	if !row.Next() {
		return nil, fmt.Errorf("session not found: %s", id)
	}

	return scanSession(row)
}

// AddTranscriptEntry adds a transcript entry
func (db *CockroachDB) AddTranscriptEntry(ctx context.Context, sessionID string, entry types.TranscriptEntry) error {
	if db.pool == nil {
		return nil
	}

	sql := `
		INSERT INTO transcripts 
		(session_id, speaker, text, audio_url, timestamp)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := db.pool.Exec(ctx, sql,
		sessionID,
		entry.Speaker,
		entry.Text,
		entry.AudioURL,
		entry.Timestamp,
	)

	if err != nil {
		db.logger.Error("Failed to save transcript entry", zap.Error(err))
		return fmt.Errorf("failed to save transcript: %w", err)
	}

	return nil
}

// GetTranscripts retrieves all transcripts for a session
func (db *CockroachDB) GetTranscripts(ctx context.Context, sessionID string) ([]types.TranscriptEntry, error) {
	if db.pool == nil {
		return nil, nil
	}

	sql := `
		SELECT speaker, text, audio_url, timestamp
		FROM transcripts
		WHERE session_id = $1
		ORDER BY timestamp ASC
	`

	rows, err := db.pool.Query(ctx, sql, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []types.TranscriptEntry
	for rows.Next() {
		var entry types.TranscriptEntry
		if err := rows.Scan(&entry.Speaker, &entry.Text, &entry.AudioURL, &entry.Timestamp); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// GetActiveSessions retrieves all active (non-terminated) sessions
func (db *CockroachDB) GetActiveSessions(ctx context.Context) ([]*types.CallSession, error) {
	if db.pool == nil {
		return nil, nil
	}

	sql := `
		SELECT id, state, caller_id, caller_number, called_number, direction,
		       start_time, end_time, duration_seconds, llm_turn_count,
		       intent, confidence, agent_id, agent_name, transfer_reason
		FROM call_sessions
		WHERE state != 'TERMINATED'
		ORDER BY start_time DESC
	`

	rows, err := db.pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*types.CallSession
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			continue
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// UpdateState updates the call state
func (db *CockroachDB) UpdateState(ctx context.Context, sessionID string, newState types.CallState) error {
	if db.pool == nil {
		return nil
	}

	sql := `UPDATE call_sessions SET state = $1, updated_at = now() WHERE id = $2`
	_, err := db.pool.Exec(ctx, sql, string(newState), sessionID)
	if err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}
	return nil
}

// HealthCheck checks database health
func (db *CockroachDB) HealthCheck(ctx context.Context) error {
	if db.pool == nil {
		return fmt.Errorf("database not initialized")
	}
	return db.pool.Ping(ctx)
}

// runMigrations executes schema migrations
func (db *CockroachDB) runMigrations(ctx context.Context) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS call_sessions (
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
		)`,
		`CREATE INDEX IF NOT EXISTS idx_calls_state ON call_sessions(state)`,
		`CREATE INDEX IF NOT EXISTS idx_calls_caller ON call_sessions(caller_id)`,
		`CREATE INDEX IF NOT EXISTS idx_calls_time ON call_sessions(start_time DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_calls_agent ON call_sessions(agent_id) WHERE agent_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS transcripts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			session_id UUID NOT NULL REFERENCES call_sessions(id) ON DELETE CASCADE,
			speaker VARCHAR(32) NOT NULL,
			text TEXT NOT NULL,
			audio_url VARCHAR(512),
			timestamp TIMESTAMPTZ DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_transcripts_session ON transcripts(session_id, timestamp DESC)`,
	}

	for i, migration := range migrations {
		_, err := db.pool.Exec(ctx, migration)
		if err != nil {
			db.logger.Error("Migration failed", zap.Int("index", i), zap.Error(err))
			return fmt.Errorf("migration %d failed: %w", i, err)
		}
	}

	db.logger.Info("CockroachDB migrations completed", zap.Int("count", len(migrations)))
	return nil
}

// buildConnString creates a PostgreSQL connection string for CockroachDB
func buildConnString(cfg *config.CockroachDBConfig) string {
	return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
		cfg.SSLMode,
	)
}

// scanSession scans a database row into a CallSession
func scanSession(row pgx.Row) (*types.CallSession, error) {
	var session types.CallSession
	var endTime *time.Time

	err := row.Scan(
		&session.ID,
		&session.State,
		&session.CallerID,
		&session.CallerNumber,
		&session.CalledNumber,
		&session.Direction,
		&session.StartTime,
		&endTime,
		&session.Duration,
		&session.LLMTurnCount,
		&session.Intent,
		&session.Confidence,
		&session.AgentID,
		&session.AgentName,
		&session.TransferReason,
	)

	if err != nil {
		return nil, err
	}

	if endTime != nil && !endTime.IsZero() {
		session.EndTime = endTime
	}

	return &session, nil
}
