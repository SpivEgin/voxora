package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"voip-server/internal/config"
)

// Client is the interface for STT clients
type Client interface {
	Transcribe(ctx context.Context, audioData []byte) (*TranscriptionResult, error)
	HealthCheck(ctx context.Context) error
	GetName() string
}

// TranscriptionResult represents STT output
type TranscriptionResult struct {
	Text       string     `json:"text"`
	Confidence float64    `json:"confidence"`
	Language   string     `json:"language"`
	Words      []WordInfo `json:"words,omitempty"`
	Duration   float64    `json:"duration"`
}

// WordInfo contains word-level information
type WordInfo struct {
	Word       string  `json:"word"`
	StartTime  float64 `json:"start_time"`
	EndTime    float64 `json:"end_time"`
	Confidence float64 `json:"confidence"`
}

// Manager manages STT clients
type Manager struct {
	primary     Client
	fallback    Client
	logger      *zap.Logger
	useFallback bool
}

// NewManager creates a new STT manager
func NewManager(cfg *config.STTConfig, logger *zap.Logger) (*Manager, error) {
	m := &Manager{
		logger: logger,
	}

	// Create primary client
	switch cfg.Engine {
	case "whisper":
		m.primary = NewWhisperClient(
			cfg.Whisper.Host,
			cfg.Whisper.Port,
			cfg.Whisper.Model,
			cfg.Whisper.Language,
			cfg.Whisper.Device,
			cfg.Whisper.ComputeType,
			logger,
		)
	case "vosk":
		m.primary = NewVoskClient(
			cfg.Vosk.Host,
			cfg.Vosk.Port,
			cfg.Vosk.ModelPath,
			cfg.Vosk.SampleRate,
			logger,
		)
	default:
		return nil, fmt.Errorf("unsupported STT engine: %s", cfg.Engine)
	}

	// Create fallback client (if different from primary)
	if cfg.Engine == "whisper" {
		m.fallback = NewVoskClient(
			cfg.Vosk.Host,
			cfg.Vosk.Port,
			cfg.Vosk.ModelPath,
			cfg.Vosk.SampleRate,
			logger,
		)
	} else {
		m.fallback = NewWhisperClient(
			cfg.Whisper.Host,
			cfg.Whisper.Port,
			cfg.Whisper.Model,
			cfg.Whisper.Language,
			cfg.Whisper.Device,
			cfg.Whisper.ComputeType,
			logger,
		)
	}

	return m, nil
}

// Transcribe performs speech-to-text with fallback
func (m *Manager) Transcribe(ctx context.Context, audioData []byte) (*TranscriptionResult, error) {
	// Try primary first
	result, err := m.primary.Transcribe(ctx, audioData)
	if err == nil {
		m.logger.Debug("STT primary transcription successful",
			zap.String("engine", m.primary.GetName()),
			zap.String("text", result.Text),
			zap.Float64("confidence", result.Confidence))
		return result, nil
	}

	m.logger.Warn("STT primary failed, using fallback",
		zap.String("primary", m.primary.GetName()),
		zap.Error(err))

	// Try fallback
	result, err = m.fallback.Transcribe(ctx, audioData)
	if err != nil {
		return nil, fmt.Errorf("both STT engines failed: %w", err)
	}

	m.logger.Debug("STT fallback transcription successful",
		zap.String("engine", m.fallback.GetName()),
		zap.String("text", result.Text),
		zap.Float64("confidence", result.Confidence))

	return result, nil
}

// HealthCheck checks both STT engines
func (m *Manager) HealthCheck(ctx context.Context) error {
	// Check primary
	if err := m.primary.HealthCheck(ctx); err != nil {
		m.logger.Warn("STT primary health check failed", zap.Error(err))
		// Check fallback
		if err := m.fallback.HealthCheck(ctx); err != nil {
			return fmt.Errorf("both STT engines unhealthy")
		}
	}
	return nil
}

// WhisperClient implements STT client for Whisper/faster-whisper
type WhisperClient struct {
	client      *http.Client
	host        string
	port        int
	model       string
	language    string
	device      string
	computeType string
	logger      *zap.Logger
}

// NewWhisperClient creates a new Whisper client
func NewWhisperClient(host string, port int, model, language, device, computeType string, logger *zap.Logger) *WhisperClient {
	return &WhisperClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		host:        host,
		port:        port,
		model:       model,
		language:    language,
		device:      device,
		computeType: computeType,
		logger:      logger,
	}
}

// Transcribe sends audio to Whisper server
func (w *WhisperClient) Transcribe(ctx context.Context, audioData []byte) (*TranscriptionResult, error) {
	url := fmt.Sprintf("http://%s:%d/transcribe", w.host, w.port)

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add audio file
	part, err := writer.CreateFormFile("audio", "audio.wav")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return nil, fmt.Errorf("failed to write audio data: %w", err)
	}

	// Add parameters
	if err := writer.WriteField("model", w.model); err != nil {
		return nil, err
	}
	if err := writer.WriteField("language", w.language); err != nil {
		return nil, err
	}
	if err := writer.WriteField("compute_type", w.computeType); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("whisper API error: %s, body: %s", resp.Status, string(body))
	}

	var whisperResp struct {
		Text       string  `json:"text"`
		Confidence float64 `json:"confidence"`
		Language   string  `json:"language"`
		Duration   float64 `json:"duration"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&whisperResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &TranscriptionResult{
		Text:       whisperResp.Text,
		Confidence: whisperResp.Confidence,
		Language:   whisperResp.Language,
		Duration:   whisperResp.Duration,
	}, nil
}

// HealthCheck checks Whisper health
func (w *WhisperClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("http://%s:%d/health", w.host, w.port)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("whisper health check failed: %s", resp.Status)
	}

	return nil
}

// GetName returns the client name
func (w *WhisperClient) GetName() string {
	return "whisper"
}

// VoskClient implements STT client for Vosk
type VoskClient struct {
	client     *http.Client
	host       string
	port       int
	modelPath  string
	sampleRate int
	logger     *zap.Logger
	websocket  *websocket.Conn
	mutex      sync.Mutex
}

// NewVoskClient creates a new Vosk client
func NewVoskClient(host string, port int, modelPath string, sampleRate int, logger *zap.Logger) *VoskClient {
	return &VoskClient{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		host:       host,
		port:       port,
		modelPath:  modelPath,
		sampleRate: sampleRate,
		logger:     logger,
	}
}

// Transcribe sends audio to Vosk server via WebSocket
func (v *VoskClient) Transcribe(ctx context.Context, audioData []byte) (*TranscriptionResult, error) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	// Ensure WebSocket connection
	if v.websocket == nil {
		if err := v.connect(ctx); err != nil {
			return nil, fmt.Errorf("failed to connect to Vosk: %w", err)
		}
	}

	// Send audio data
	if err := v.websocket.WriteMessage(websocket.BinaryMessage, audioData); err != nil {
		// Try to reconnect once
		v.websocket.Close()
		v.websocket = nil
		if err := v.connect(ctx); err != nil {
			return nil, fmt.Errorf("failed to reconnect to Vosk: %w", err)
		}
		if err := v.websocket.WriteMessage(websocket.BinaryMessage, audioData); err != nil {
			return nil, fmt.Errorf("failed to send audio to Vosk: %w", err)
		}
	}

	// Send end-of-stream marker
	if err := v.websocket.WriteMessage(websocket.TextMessage, []byte(`{"eof" : 1}`)); err != nil {
		return nil, fmt.Errorf("failed to send EOF to Vosk: %w", err)
	}

	// Read response
	_, message, err := v.websocket.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read from Vosk: %w", err)
	}

	var voskResp struct {
		Text   string `json:"text"`
		Result []struct {
			Conf  float64 `json:"conf"`
			Start float64 `json:"start"`
			End   float64 `json:"end"`
			Word  string  `json:"word"`
		} `json:"result,omitempty"`
	}

	if err := json.Unmarshal(message, &voskResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Vosk response: %w", err)
	}

	// Calculate confidence and duration
	var confidence float64
	var words []WordInfo
	duration := 0.0

	if len(voskResp.Result) > 0 {
		totalConf := 0.0
		for _, r := range voskResp.Result {
			totalConf += r.Conf
			words = append(words, WordInfo{
				Word:       r.Word,
				StartTime:  r.Start,
				EndTime:    r.End,
				Confidence: r.Conf,
			})
		}
		confidence = totalConf / float64(len(voskResp.Result))
		duration = voskResp.Result[len(voskResp.Result)-1].End
	}

	return &TranscriptionResult{
		Text:       voskResp.Text,
		Confidence: confidence,
		Words:      words,
		Duration:   duration,
	}, nil
}

// connect establishes WebSocket connection to Vosk
func (v *VoskClient) connect(ctx context.Context) error {
	wsURL := fmt.Sprintf("ws://%s:%d", v.host, v.port)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to dial Vosk: %w", err)
	}

	v.websocket = ws
	return nil
}

// HealthCheck checks Vosk health
func (v *VoskClient) HealthCheck(ctx context.Context) error {
	// Try to establish connection
	if err := v.connect(ctx); err != nil {
		return err
	}

	// Close connection after check
	if v.websocket != nil {
		v.websocket.Close()
		v.websocket = nil
	}

	return nil
}

// GetName returns the client name
func (v *VoskClient) GetName() string {
	return "vosk"
}

// MockClient is a mock STT client for testing
type MockClient struct {
	mockText string
}

// NewMockClient creates a new mock STT client
func NewMockClient(text string) *MockClient {
	return &MockClient{mockText: text}
}

// Transcribe returns mock transcription
func (m *MockClient) Transcribe(ctx context.Context, audioData []byte) (*TranscriptionResult, error) {
	return &TranscriptionResult{
		Text:       m.mockText,
		Confidence: 0.95,
		Language:   "en",
	}, nil
}

// HealthCheck always returns nil
func (m *MockClient) HealthCheck(ctx context.Context) error {
	return nil
}

// GetName returns the client name
func (m *MockClient) GetName() string {
	return "mock"
}
