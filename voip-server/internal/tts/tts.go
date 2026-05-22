package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
	"voip-server/internal/config"
)

// Client is the interface for TTS clients
type Client interface {
	Synthesize(ctx context.Context, text string) (*SynthesisResult, error)
	HealthCheck(ctx context.Context) error
	GetName() string
}

// SynthesisResult represents TTS output
type SynthesisResult struct {
	AudioData []byte  `json:"audio_data"`
	Format    string  `json:"format"` // wav, mp3, opus
	SampleRate int    `json:"sample_rate"`
	Duration  float64 `json:"duration"`
}

// Manager manages TTS clients
type Manager struct {
	primary Client
	logger  *zap.Logger
}

// NewManager creates a new TTS manager
func NewManager(cfg *config.TTSConfig, logger *zap.Logger) (*Manager, error) {
	m := &Manager{
		logger: logger,
	}

	switch cfg.Engine {
	case "piper":
		m.primary = NewPiperClient(
			cfg.Piper.Host,
			cfg.Piper.Port,
			cfg.Piper.Voice,
			cfg.Piper.Speed,
			logger,
		)
	case "coqui":
		m.primary = NewCoquiClient(
			cfg.Coqui.Host,
			cfg.Coqui.Port,
			cfg.Coqui.Model,
			logger,
		)
	case "kokoro":
		m.primary = NewKokoroClient(
			cfg.Kokoro.Host,
			cfg.Kokoro.Port,
			cfg.Kokoro.Voice,
			logger,
		)
	default:
		return nil, fmt.Errorf("unsupported TTS engine: %s", cfg.Engine)
	}

	return m, nil
}

// Synthesize converts text to speech
func (m *Manager) Synthesize(ctx context.Context, text string) (*SynthesisResult, error) {
	result, err := m.primary.Synthesize(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("TTS synthesis failed: %w", err)
	}

	m.logger.Debug("TTS synthesis successful",
		zap.String("engine", m.primary.GetName()),
		zap.Int("audio_size", len(result.AudioData)),
		zap.Float64("duration", result.Duration))

	return result, nil
}

// HealthCheck checks TTS health
func (m *Manager) HealthCheck(ctx context.Context) error {
	return m.primary.HealthCheck(ctx)
}

// PiperClient implements TTS client for Piper
type PiperClient struct {
	client *http.Client
	host   string
	port   int
	voice  string
	speed  float64
	logger *zap.Logger
}

// NewPiperClient creates a new Piper client
func NewPiperClient(host string, port int, voice string, speed float64, logger *zap.Logger) *PiperClient {
	return &PiperClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		host:   host,
		port:   port,
		voice:  voice,
		speed:  speed,
		logger: logger,
	}
}

// Synthesize sends text to Piper server
func (p *PiperClient) Synthesize(ctx context.Context, text string) (*SynthesisResult, error) {
	url := fmt.Sprintf("http://%s:%d/synthesize", p.host, p.port)

	requestBody := map[string]interface{}{
		"text":  text,
		"voice": p.voice,
		"speed": p.speed,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("piper API error: %s, body: %s", resp.Status, string(body))
	}

	// Read audio data
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio data: %w", err)
	}

	// Piper returns WAV format
	return &SynthesisResult{
		AudioData:  audioData,
		Format:     "wav",
		SampleRate: 22050,
		Duration:   float64(len(audioData)) / (22050 * 2), // Approximate for 16-bit mono
	}, nil
}

// HealthCheck checks Piper health
func (p *PiperClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("http://%s:%d/health", p.host, p.port)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("piper health check failed: %s", resp.Status)
	}

	return nil
}

// GetName returns the client name
func (p *PiperClient) GetName() string {
	return "piper"
}

// CoquiClient implements TTS client for Coqui
type CoquiClient struct {
	client *http.Client
	host   string
	port   int
	model  string
	logger *zap.Logger
}

// NewCoquiClient creates a new Coqui client
func NewCoquiClient(host string, port int, model string, logger *zap.Logger) *CoquiClient {
	return &CoquiClient{
		client: &http.Client{
			Timeout: 60 * time.Second, // Coqui can be slower
		},
		host:   host,
		port:   port,
		model:  model,
		logger: logger,
	}
}

// Synthesize sends text to Coqui server
func (c *CoquiClient) Synthesize(ctx context.Context, text string) (*SynthesisResult, error) {
	url := fmt.Sprintf("http://%s:%d/api/tts", c.host, c.port)

	requestBody := map[string]interface{}{
		"text": text,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("coqui API error: %s, body: %s", resp.Status, string(body))
	}

	// Read audio data
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio data: %w", err)
	}

	return &SynthesisResult{
		AudioData:  audioData,
		Format:     "wav",
		SampleRate: 22050,
		Duration:   float64(len(audioData)) / (22050 * 2),
	}, nil
}

// HealthCheck checks Coqui health
func (c *CoquiClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("http://%s:%d/", c.host, c.port)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("coqui health check failed: %s", resp.Status)
	}

	return nil
}

// GetName returns the client name
func (c *CoquiClient) GetName() string {
	return "coqui"
}

// KokoroClient implements TTS client for Kokoro
type KokoroClient struct {
	client *http.Client
	host   string
	port   int
	voice  string
	logger *zap.Logger
}

// NewKokoroClient creates a new Kokoro client
func NewKokoroClient(host string, port int, voice string, logger *zap.Logger) *KokoroClient {
	return &KokoroClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		host:   host,
		port:   port,
		voice:  voice,
		logger: logger,
	}
}

// Synthesize sends text to Kokoro server
func (k *KokoroClient) Synthesize(ctx context.Context, text string) (*SynthesisResult, error) {
	url := fmt.Sprintf("http://%s:%d/synthesize", k.host, k.port)

	requestBody := map[string]interface{}{
		"text":  text,
		"voice": k.voice,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := k.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kokoro API error: %s, body: %s", resp.Status, string(body))
	}

	// Read audio data
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio data: %w", err)
	}

	return &SynthesisResult{
		AudioData:  audioData,
		Format:     "mp3",
		SampleRate: 24000,
		Duration:   float64(len(audioData)) / (24000 * 2),
	}, nil
}

// HealthCheck checks Kokoro health
func (k *KokoroClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("http://%s:%d/health", k.host, k.port)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := k.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("kokoro health check failed: %s", resp.Status)
	}

	return nil
}

// GetName returns the client name
func (k *KokoroClient) GetName() string {
	return "kokoro"
}

// MockClient is a mock TTS client for testing
type MockClient struct {
	mockAudio []byte
}

// NewMockClient creates a new mock TTS client
func NewMockClient(audio []byte) *MockClient {
	return &MockClient{mockAudio: audio}
}

// Synthesize returns mock audio
func (m *MockClient) Synthesize(ctx context.Context, text string) (*SynthesisResult, error) {
	return &SynthesisResult{
		AudioData:  m.mockAudio,
		Format:     "wav",
		SampleRate: 16000,
		Duration:   1.0,
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
