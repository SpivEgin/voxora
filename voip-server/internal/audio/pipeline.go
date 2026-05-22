package audio

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"
	"voip-server/internal/config"
	"voip-server/internal/llm"
	"voip-server/internal/state"
	"voip-server/internal/stt"
	"voip-server/internal/tts"
	"voip-server/internal/transfer"
)

// Pipeline manages the audio processing pipeline
type Pipeline struct {
	config       *config.AudioConfig
	sttManager   *stt.Manager
	ttsManager   *tts.Manager
	llmManager   *llm.Manager
	transfer     *transfer.Engine
	callManager  *state.CallManager
	logger       *zap.Logger
	
	sessions     map[string]*AudioSession
	mutex        sync.RWMutex
}

// AudioSession represents an audio session for a call
type AudioSession struct {
	SessionID       string
	Buffer          []byte
	Transcript      []string
	LastActivity    time.Time
	SilenceFrames   int
	IsSpeaking      bool
	CurrentAudio    []byte
	
	ctx             context.Context
	cancel          context.CancelFunc
	inputChan       chan []byte
	outputChan      chan []byte
	mutex           sync.RWMutex
}

// NewPipeline creates a new audio pipeline
func NewPipeline(
	cfg *config.AudioConfig,
	sttManager *stt.Manager,
	ttsManager *tts.Manager,
	llmManager *llm.Manager,
	transferEngine *transfer.Engine,
	callManager *state.CallManager,
	logger *zap.Logger,
) *Pipeline {
	return &Pipeline{
		config:      cfg,
		sttManager:  sttManager,
		ttsManager:  ttsManager,
		llmManager:  llmManager,
		transfer:    transferEngine,
		callManager: callManager,
		logger:      logger,
		sessions:    make(map[string]*AudioSession),
	}
}

// CreateSession creates a new audio session
func (p *Pipeline) CreateSession(sessionID string) (*AudioSession, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if _, exists := p.sessions[sessionID]; exists {
		return nil, fmt.Errorf("audio session already exists: %s", sessionID)
	}

	ctx, cancel := context.WithCancel(context.Background())
	
	session := &AudioSession{
		SessionID:    sessionID,
		Buffer:       make([]byte, 0, p.config.BufferSize*100), // Pre-allocate
		Transcript:   make([]string, 0),
		LastActivity: time.Now(),
		ctx:          ctx,
		cancel:       cancel,
		inputChan:    make(chan []byte, 100),
		outputChan:   make(chan []byte, 100),
	}

	p.sessions[sessionID] = session

	// Start processing goroutines
	go p.processAudioInput(session)
	go p.processLLMResponses(session)

	p.logger.Info("Audio session created",
		zap.String("session_id", sessionID))

	return session, nil
}

// GetSession returns an audio session
func (p *Pipeline) GetSession(sessionID string) (*AudioSession, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	session, exists := p.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("audio session not found: %s", sessionID)
	}

	return session, nil
}

// CloseSession closes an audio session
func (p *Pipeline) CloseSession(sessionID string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	session, exists := p.sessions[sessionID]
	if !exists {
		return fmt.Errorf("audio session not found: %s", sessionID)
	}

	// Cancel context and close channels
	session.cancel()
	close(session.inputChan)
	close(session.outputChan)

	delete(p.sessions, sessionID)

	p.logger.Info("Audio session closed",
		zap.String("session_id", sessionID))

	return nil
}

// WriteAudio writes audio data to a session
func (p *Pipeline) WriteAudio(sessionID string, data []byte) error {
	session, err := p.GetSession(sessionID)
	if err != nil {
		return err
	}

	select {
	case session.inputChan <- data:
		return nil
	case <-time.After(100 * time.Millisecond):
		return fmt.Errorf("audio input channel full")
	}
}

// ReadAudio reads synthesized audio from a session
func (p *Pipeline) ReadAudio(sessionID string) ([]byte, error) {
	session, err := p.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	select {
	case data := <-session.outputChan:
		return data, nil
	case <-time.After(100 * time.Millisecond):
		return nil, fmt.Errorf("no audio available")
	}
}

// processAudioInput processes incoming audio
func (p *Pipeline) processAudioInput(session *AudioSession) {
	for {
		select {
		case <-session.ctx.Done():
			return
		case data, ok := <-session.inputChan:
			if !ok {
				return
			}

			// Append to buffer
			session.mutex.Lock()
			session.Buffer = append(session.Buffer, data...)
			session.LastActivity = time.Now()
			session.mutex.Unlock()

			// Check for silence
			if p.config.SilenceDetection.Enabled {
				if p.isSilence(data) {
					session.SilenceFrames++
					
					// Process on silence after hangover
					if session.SilenceFrames >= p.config.SilenceDetection.Hangover {
						if session.IsSpeaking {
							// End of utterance, process
							go p.processUtterance(session)
							session.IsSpeaking = false
						}
					}
				} else {
					session.SilenceFrames = 0
					session.IsSpeaking = true
				}
			}
		}
	}
}

// processUtterance processes a complete utterance
func (p *Pipeline) processUtterance(session *AudioSession) {
	session.mutex.Lock()
	if len(session.Buffer) == 0 {
		session.mutex.Unlock()
		return
	}

	// Copy buffer
	audioData := make([]byte, len(session.Buffer))
	copy(audioData, session.Buffer)
	session.Buffer = session.Buffer[:0] // Clear but keep capacity
	session.mutex.Unlock()

	// Transcribe
	result, err := p.sttManager.Transcribe(session.ctx, audioData)
	if err != nil {
		p.logger.Error("STT failed",
			zap.String("session_id", session.SessionID),
			zap.Error(err))
		return
	}

	if result.Text == "" {
		return // No speech detected
	}

	p.logger.Debug("Transcription complete",
		zap.String("session_id", session.SessionID),
		zap.String("text", result.Text),
		zap.Float64("confidence", result.Confidence))

	// Add to transcript
	session.mutex.Lock()
	session.Transcript = append(session.Transcript, result.Text)
	transcriptCopy := make([]string, len(session.Transcript))
	copy(transcriptCopy, session.Transcript)
	session.mutex.Unlock()

	// Add transcript entry to call session
	entry := state.TranscriptEntry{
		Timestamp: time.Now(),
		Speaker:   "caller",
		Text:      result.Text,
	}
	if err := p.callManager.AddTranscriptEntry(session.ctx, session.SessionID, entry); err != nil {
		p.logger.Error("Failed to add transcript entry",
			zap.String("session_id", session.SessionID),
			zap.Error(err))
	}

	// Get LLM response
	llmResult, err := p.llmManager.Complete(session.ctx, transcriptCopy)
	if err != nil {
		p.logger.Error("LLM completion failed",
			zap.String("session_id", session.SessionID),
			zap.Error(err))
		return
	}

	// Update LLM response in call session
	if err := p.callManager.UpdateLLMResponse(session.ctx, session.SessionID,
		llmResult.Response, llmResult.Confidence, llmResult.Intent); err != nil {
		p.logger.Error("Failed to update LLM response",
			zap.String("session_id", session.SessionID),
			zap.Error(err))
	}

	// Check transfer conditions
	conditionResult, err := p.transfer.Evaluate(session.ctx, session.SessionID, llmResult, transcriptCopy)
	if err != nil {
		p.logger.Error("Transfer evaluation failed",
			zap.String("session_id", session.SessionID),
			zap.Error(err))
	}

	if conditionResult.ShouldTransfer {
		// Request transfer
		if err := p.transfer.RequestTransfer(session.ctx, session.SessionID,
			conditionResult.Target, conditionResult.Reason); err != nil {
			p.logger.Error("Transfer request failed",
				zap.String("session_id", session.SessionID),
				zap.Error(err))
		}
		return
	}

	// Add LLM response to transcript
	entry = state.TranscriptEntry{
		Timestamp: time.Now(),
		Speaker:   "llm",
		Text:      llmResult.Response,
	}
	if err := p.callManager.AddTranscriptEntry(session.ctx, session.SessionID, entry); err != nil {
		p.logger.Error("Failed to add LLM transcript entry",
			zap.String("session_id", session.SessionID),
			zap.Error(err))
	}

	// Synthesize response
	ttsResult, err := p.ttsManager.Synthesize(session.ctx, llmResult.Response)
	if err != nil {
		p.logger.Error("TTS synthesis failed",
			zap.String("session_id", session.SessionID),
			zap.Error(err))
		return
	}

	// Send to output
	select {
	case session.outputChan <- ttsResult.AudioData:
		p.logger.Debug("Audio synthesized and queued",
			zap.String("session_id", session.SessionID),
			zap.Int("audio_size", len(ttsResult.AudioData)))
	case <-time.After(100 * time.Millisecond):
		p.logger.Warn("Output channel full, dropping audio",
			zap.String("session_id", session.SessionID))
	}
}

// processLLMResponses processes LLM responses (placeholder for streaming)
func (p *Pipeline) processLLMResponses(session *AudioSession) {
	// This could be used for streaming responses
	// For now, we process synchronously in processUtterance
}

// isSilence detects if audio data is silence
func (p *Pipeline) isSilence(data []byte) bool {
	// Simple energy-based silence detection
	// In production, use proper audio processing
	
	if len(data) < 2 {
		return true
	}

	// Calculate RMS energy (simplified)
	var sum float64
	for i := 0; i < len(data)-1; i += 2 {
		sample := int16(data[i]) | int16(data[i+1])<<8
		sum += float64(sample * sample)
	}

	rms := sum / float64(len(data)/2)
	db := 10 * math.Log10(rms)

	return db < float64(p.config.SilenceDetection.Threshold)
}

// GetActiveSessions returns count of active audio sessions
func (p *Pipeline) GetActiveSessions() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return len(p.sessions)
}

// Cleanup removes inactive sessions
func (p *Pipeline) Cleanup() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	timeout := time.Duration(p.config.SilenceDetection.Hangover*20) * time.Millisecond
	
	for id, session := range p.sessions {
		session.mutex.RLock()
		inactive := time.Since(session.LastActivity) > timeout
		session.mutex.RUnlock()
		
		if inactive {
			session.cancel()
			delete(p.sessions, id)
		}
	}
}


