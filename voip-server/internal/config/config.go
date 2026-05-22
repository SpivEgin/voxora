package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config represents the complete application configuration
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Webhooks WebhooksConfig `mapstructure:"webhooks"`
	STT      STTConfig      `mapstructure:"stt"`
	LLM      LLMConfig      `mapstructure:"llm"`
	TTS      TTSConfig      `mapstructure:"tts"`
	Transfer TransferConfig `mapstructure:"transfer"`
	Agents   AgentsConfig   `mapstructure:"agents"`
	Audio    AudioConfig    `mapstructure:"audio"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Metrics  MetricsConfig  `mapstructure:"metrics"`
	Security SecurityConfig `mapstructure:"security"`
	Test     TestConfig     `mapstructure:"test"`
}

// ServerConfig contains server-related settings
type ServerConfig struct {
	Name        string    `mapstructure:"name"`
	Version     string    `mapstructure:"version"`
	Environment string    `mapstructure:"environment"`
	SIP         SIPConfig `mapstructure:"sip"`
	WebSocket   WSConfig  `mapstructure:"websocket"`
	REST        RESTConfig `mapstructure:"rest"`
	gRPC        GRPCConfig `mapstructure:"grpc"`
}

// SIPConfig contains SIP server settings
type SIPConfig struct {
	Enabled    bool       `mapstructure:"enabled"`
	Host       string     `mapstructure:"host"`
	Port       int        `mapstructure:"port"`
	TLSPort    int        `mapstructure:"tls_port"`
	TLSEnabled bool       `mapstructure:"tls_enabled"`
	Domain     string     `mapstructure:"domain"`
	UserAgent  string     `mapstructure:"user_agent"`
	Auth       AuthConfig `mapstructure:"auth"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Realm    string `mapstructure:"realm"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

// WSConfig contains WebSocket settings
type WSConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Host    string `mapstructure:"host"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// RESTConfig contains REST API settings
type RESTConfig struct {
	Enabled    bool `mapstructure:"enabled"`
	Host       string `mapstructure:"host"`
	Port       int    `mapstructure:"port"`
	TLSEnabled bool   `mapstructure:"tls_enabled"`
}

// GRPCConfig contains gRPC settings
type GRPCConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Host       string `mapstructure:"host"`
	Port       int    `mapstructure:"port"`
	TLSEnabled bool   `mapstructure:"tls_enabled"`
}

// RedisConfig contains Redis settings
type RedisConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	Password     string `mapstructure:"password"`
	Database     int    `mapstructure:"database"`
	PoolSize     int    `mapstructure:"pool_size"`
	MinIdleConns int    `mapstructure:"min_idle_conns"`
}

// WebhooksConfig contains webhook settings
type WebhooksConfig struct {
	Enabled   bool             `mapstructure:"enabled"`
	Providers []ProviderConfig `mapstructure:"providers"`
}

// ProviderConfig contains webhook provider settings
type ProviderConfig struct {
	Name       string `mapstructure:"name"`
	AccountSID string `mapstructure:"account_sid"`
	AuthToken  string `mapstructure:"auth_token"`
	WebhookURL string `mapstructure:"webhook_url"`
	ProjectID  string `mapstructure:"project_id"`
	APIToken   string `mapstructure:"api_token"`
}

// STTConfig contains speech-to-text settings
type STTConfig struct {
	Engine  string         `mapstructure:"engine"`
	Whisper WhisperConfig  `mapstructure:"whisper"`
	Vosk    VoskConfig     `mapstructure:"vosk"`
}

// WhisperConfig contains Whisper settings
type WhisperConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	Model        string `mapstructure:"model"`
	Language     string `mapstructure:"language"`
	Device       string `mapstructure:"device"`
	ComputeType  string `mapstructure:"compute_type"`
}

// VoskConfig contains Vosk settings
type VoskConfig struct {
	Host       string `mapstructure:"host"`
	Port       int    `mapstructure:"port"`
	ModelPath  string `mapstructure:"model_path"`
	SampleRate int    `mapstructure:"sample_rate"`
}

// LLMConfig contains LLM settings
type LLMConfig struct {
	Backend string        `mapstructure:"backend"`
	Ollama  OllamaConfig  `mapstructure:"ollama"`
	VLLM    VLLMConfig    `mapstructure:"vllm"`
	LocalAI LocalAIConfig `mapstructure:"localai"`
}

// OllamaConfig contains Ollama settings
type OllamaConfig struct {
	Host          string `mapstructure:"host"`
	Port          int    `mapstructure:"port"`
	Model         string `mapstructure:"model"`
	Temperature   float64 `mapstructure:"temperature"`
	MaxTokens     int    `mapstructure:"max_tokens"`
	Timeout       int    `mapstructure:"timeout"`
	SystemPrompt  string `mapstructure:"system_prompt"`
}

// VLLMConfig contains vLLM settings
type VLLMConfig struct {
	Host   string `mapstructure:"host"`
	Port   int    `mapstructure:"port"`
	Model  string `mapstructure:"model"`
}

// LocalAIConfig contains LocalAI settings
type LocalAIConfig struct {
	Host  string `mapstructure:"host"`
	Port  int    `mapstructure:"port"`
	Model string `mapstructure:"model"`
}

// TTSConfig contains text-to-speech settings
type TTSConfig struct {
	Engine string       `mapstructure:"engine"`
	Piper  PiperConfig  `mapstructure:"piper"`
	Coqui  CoquiConfig  `mapstructure:"coqui"`
	Kokoro KokoroConfig `mapstructure:"kokoro"`
}

// PiperConfig contains Piper TTS settings
type PiperConfig struct {
	Host   string  `mapstructure:"host"`
	Port   int     `mapstructure:"port"`
	Voice  string  `mapstructure:"voice"`
	Speed  float64 `mapstructure:"speed"`
}

// CoquiConfig contains Coqui TTS settings
type CoquiConfig struct {
	Host  string `mapstructure:"host"`
	Port  int    `mapstructure:"port"`
	Model string `mapstructure:"model"`
}

// KokoroConfig contains Kokoro TTS settings
type KokoroConfig struct {
	Host  string `mapstructure:"host"`
	Port  int    `mapstructure:"port"`
	Voice string `mapstructure:"voice"`
}

// TransferConfig contains call transfer settings
type TransferConfig struct {
	Enabled    bool               `mapstructure:"enabled"`
	Conditions TransferConditions `mapstructure:"conditions"`
	Targets    []TransferTarget   `mapstructure:"targets"`
}

// TransferConditions contains transfer condition rules
type TransferConditions struct {
	CallerRequestsHuman bool     `mapstructure:"caller_requests_human"`
	ConfidenceThreshold float64  `mapstructure:"confidence_threshold"`
	IntentTriggers      []string `mapstructure:"intent_triggers"`
	MaxLLMTurns         int      `mapstructure:"max_llm_turns"`
	MaxCallDuration     int      `mapstructure:"max_call_duration"`
	AbuseDetection      bool     `mapstructure:"abuse_detection"`
	SilenceTimeout      int      `mapstructure:"silence_timeout"`
}

// TransferTarget contains transfer destination settings
type TransferTarget struct {
	Name         string `mapstructure:"name"`
	SIPURI       string `mapstructure:"sip_uri"`
	Priority     int    `mapstructure:"priority"`
	Available24H bool   `mapstructure:"available_24h"`
}

// AgentsConfig contains agent settings
type AgentsConfig struct {
	Queue QueueConfig `mapstructure:"queue"`
	Auth  AgentAuthConfig `mapstructure:"auth"`
}

// QueueConfig contains queue settings
type QueueConfig struct {
	Name         string `mapstructure:"name"`
	MaxWaitTime  int    `mapstructure:"max_wait_time"`
}

// AgentAuthConfig contains agent authentication settings
type AgentAuthConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	JWTSecret    string `mapstructure:"jwt_secret"`
	TokenExpiry  int    `mapstructure:"token_expiry"`
}

// AudioConfig contains audio pipeline settings
type AudioConfig struct {
	SampleRate       int      `mapstructure:"sample_rate"`
	FrameDuration    int      `mapstructure:"frame_duration"`
	Codecs           []string `mapstructure:"codecs"`
	BufferSize       int      `mapstructure:"buffer_size"`
	SilenceDetection SilenceConfig `mapstructure:"silence_detection"`
}

// SilenceConfig contains silence detection settings
type SilenceConfig struct {
	Enabled   bool `mapstructure:"enabled"`
	Threshold int  `mapstructure:"threshold"`
	Hangover  int  `mapstructure:"hangover"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
	File   string `mapstructure:"file"`
}

// MetricsConfig contains metrics settings
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// SecurityConfig contains security settings
type SecurityConfig struct {
	TLS        TLSConfig       `mapstructure:"tls"`
	SRTP       SRTPConfig      `mapstructure:"srtp"`
	RateLimit  RateLimitConfig `mapstructure:"rate_limit"`
}

// TLSConfig contains TLS settings
type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

// SRTPConfig contains SRTP settings
type SRTPConfig struct {
	Enabled      bool     `mapstructure:"enabled"`
	CryptoSuites []string `mapstructure:"crypto_suites"`
}

// RateLimitConfig contains rate limiting settings
type RateLimitConfig struct {
	Enabled            bool `mapstructure:"enabled"`
	RequestsPerSecond  int  `mapstructure:"requests_per_second"`
	Burst              int  `mapstructure:"burst"`
}

// TestConfig contains test settings
type TestConfig struct {
	MockServices bool   `mapstructure:"mock_services"`
	TestAudio    string `mapstructure:"test_audio"`
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Set defaults
	setDefaults()

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Override with environment variables
	viper.SetEnvPrefix("VOIP")
	viper.AutomaticEnv()

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := validate(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// setDefaults sets default configuration values
func setDefaults() {
	viper.SetDefault("server.sip.port", 5060)
	viper.SetDefault("server.sip.tls_port", 5061)
	viper.SetDefault("server.websocket.port", 8080)
	viper.SetDefault("server.rest.port", 8080)
	viper.SetDefault("server.grpc.port", 50051)
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("stt.whisper.port", 9090)
	viper.SetDefault("stt.vosk.port", 2700)
	viper.SetDefault("llm.ollama.port", 11434)
	viper.SetDefault("llm.vllm.port", 8000)
	viper.SetDefault("llm.localai.port", 8080)
	viper.SetDefault("tts.piper.port", 5000)
	viper.SetDefault("tts.coqui.port", 5002)
	viper.SetDefault("tts.kokoro.port", 8880)
	viper.SetDefault("audio.sample_rate", 16000)
	viper.SetDefault("audio.frame_duration", 20)
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "console")
}

// validate validates the configuration
func validate(config *Config) error {
	if config.Server.SIP.Enabled && config.Server.SIP.Port <= 0 {
		return fmt.Errorf("SIP port must be positive")
	}

	if config.STT.Engine != "whisper" && config.STT.Engine != "vosk" {
		return fmt.Errorf("invalid STT engine: %s", config.STT.Engine)
	}

	if config.LLM.Backend != "ollama" && config.LLM.Backend != "vllm" && config.LLM.Backend != "localai" {
		return fmt.Errorf("invalid LLM backend: %s", config.LLM.Backend)
	}

	if config.TTS.Engine != "piper" && config.TTS.Engine != "coqui" && config.TTS.Engine != "kokoro" {
		return fmt.Errorf("invalid TTS engine: %s", config.TTS.Engine)
	}

	if config.Transfer.Conditions.ConfidenceThreshold < 0 || config.Transfer.Conditions.ConfidenceThreshold > 1 {
		return fmt.Errorf("confidence threshold must be between 0 and 1")
	}

	return nil
}

// GetConfigPath returns the default config file path
func GetConfigPath() string {
	// Check for config in standard locations
	paths := []string{
		"./configs/config.yaml",
		"./config.yaml",
		"/etc/voip-server/config.yaml",
		filepath.Join(os.Getenv("HOME"), ".config", "voip-server", "config.yaml"),
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return "./configs/config.yaml"
}
