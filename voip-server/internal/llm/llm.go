package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"voip-server/internal/config"
	"go.uber.org/zap"
)

// Client is the interface for LLM clients
type Client interface {
	Complete(ctx context.Context, messages []Message) (*CompletionResult, error)
	HealthCheck(ctx context.Context) error
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"` // system, user, assistant
	Content string `json:"content"`
}

// CompletionResult represents LLM output
type CompletionResult struct {
	Response       string  `json:"response"`
	TransferToAgent bool   `json:"transfer_to_agent"`
	Reason         string  `json:"reason"`
	Confidence     float64 `json:"confidence"`
	Intent         string  `json:"intent"`
	RawResponse    string  `json:"raw_response"`
}

// Manager manages LLM clients
type Manager struct {
	client Client
	logger *zap.Logger
}

// NewManager creates a new LLM manager
func NewManager(cfg *config.LLMConfig, logger *zap.Logger) (*Manager, error) {
	m := &Manager{
		logger: logger,
	}

	switch cfg.Backend {
	case "ollama":
		m.client = NewOllamaClient(&cfg.Ollama,
			cfg.Ollama.Host,
			cfg.Ollama.Port,
			logger,
		)
	case "vllm":
		m.client = NewVLLMClient(
			cfg.VLLM.Host,
			cfg.VLLM.Port,
			cfg.VLLM.Model,
			logger,
		)
	case "localai":
		m.client = NewLocalAIClient(
			cfg.LocalAI.Host,
			cfg.LocalAI.Port,
			cfg.LocalAI.Model,
			logger,
		)
	default:
		return nil, fmt.Errorf("unsupported LLM backend: %s", cfg.Backend)
	}

	return m, nil
}

// Complete sends a completion request to the LLM
func (m *Manager) Complete(ctx context.Context, transcript []string) (*CompletionResult, error) {
	// Build conversation history
	messages := []Message{
		{
			Role:    "system",
			Content: getSystemPrompt(),
		},
	}

	// Add transcript as conversation
	for i, text := range transcript {
		if i%2 == 0 {
			messages = append(messages, Message{
				Role:    "user",
				Content: text,
			})
		} else {
			messages = append(messages, Message{
				Role:    "assistant",
				Content: text,
			})
		}
	}

	result, err := m.client.Complete(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	m.logger.Debug("LLM completion successful",
		zap.String("intent", result.Intent),
		zap.Float64("confidence", result.Confidence),
		zap.Bool("transfer", result.TransferToAgent))

	return result, nil
}

// CompleteWithContext sends a completion request with additional context
func (m *Manager) CompleteWithContext(ctx context.Context, callerInfo, intent string, transcript []string) (*CompletionResult, error) {
	messages := []Message{
		{
			Role:    "system",
			Content: getSystemPrompt(),
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Caller info: %s\nCurrent intent: %s\n\nConversation history:", callerInfo, intent),
		},
	}

	// Add transcript
	for i, text := range transcript {
		if i%2 == 0 {
			messages = append(messages, Message{
				Role:    "user",
				Content: text,
			})
		} else {
			messages = append(messages, Message{
				Role:    "assistant",
				Content: text,
			})
		}
	}

	result, err := m.client.Complete(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	return result, nil
}

// HealthCheck checks LLM health
func (m *Manager) HealthCheck(ctx context.Context) error {
	return m.client.HealthCheck(ctx)
}

// OllamaClient implements LLM client for Ollama
type OllamaClient struct {
	config *config.OllamaConfig
	client *http.Client
	host   string
	port   int
	logger *zap.Logger
}

// NewOllamaClient creates a new Ollama client
func NewOllamaClient(cfg *config.OllamaConfig, host string, port int, logger *zap.Logger) *OllamaClient {
	return &OllamaClient{
		config: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		host:   host,
		port:   port,
		logger: logger,
	}
}

// Complete sends a completion request to Ollama
func (o *OllamaClient) Complete(ctx context.Context, messages []Message) (*CompletionResult, error) {
	// Build prompt from messages
	prompt := buildPromptFromMessages(messages)

	url := fmt.Sprintf("http://%s:%d/api/generate", o.host, o.port)

	requestBody := map[string]interface{}{
		"model":  o.config.Model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]interface{}{
			"temperature": o.config.Temperature,
			"num_predict": o.config.MaxTokens,
		},
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

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama API error: %s, body: %s", resp.Status, string(body))
	}

	var ollamaResp struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return parseLLMResponse(ollamaResp.Response), nil
}

// HealthCheck checks Ollama health
func (o *OllamaClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("http://%s:%d/api/tags", o.host, o.port)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama health check failed: %s", resp.Status)
	}

	return nil
}

// VLLMClient implements LLM client for vLLM
type VLLMClient struct {
	client *http.Client
	host   string
	port   int
	model  string
	logger *zap.Logger
}

// NewVLLMClient creates a new vLLM client
func NewVLLMClient(host string, port int, model string, logger *zap.Logger) *VLLMClient {
	return &VLLMClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		host:   host,
		port:   port,
		model:  model,
		logger: logger,
	}
}

// Complete sends a completion request to vLLM
func (v *VLLMClient) Complete(ctx context.Context, messages []Message) (*CompletionResult, error) {
	url := fmt.Sprintf("http://%s:%d/v1/chat/completions", v.host, v.port)

	requestBody := map[string]interface{}{
		"model":    v.model,
		"messages": messages,
		"max_tokens": 512,
		"temperature": 0.7,
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

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vllm API error: %s, body: %s", resp.Status, string(body))
	}

	var vllmResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&vllmResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(vllmResp.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned")
	}

	return parseLLMResponse(vllmResp.Choices[0].Message.Content), nil
}

// HealthCheck checks vLLM health
func (v *VLLMClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("http://%s:%d/health", v.host, v.port)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vllm health check failed: %s", resp.Status)
	}

	return nil
}

// LocalAIClient implements LLM client for LocalAI
type LocalAIClient struct {
	client *http.Client
	host   string
	port   int
	model  string
	logger *zap.Logger
}

// NewLocalAIClient creates a new LocalAI client
func NewLocalAIClient(host string, port int, model string, logger *zap.Logger) *LocalAIClient {
	return &LocalAIClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		host:   host,
		port:   port,
		model:  model,
		logger: logger,
	}
}

// Complete sends a completion request to LocalAI
func (l *LocalAIClient) Complete(ctx context.Context, messages []Message) (*CompletionResult, error) {
	url := fmt.Sprintf("http://%s:%d/v1/chat/completions", l.host, l.port)

	requestBody := map[string]interface{}{
		"model":    l.model,
		"messages": messages,
		"max_tokens": 512,
		"temperature": 0.7,
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

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("localai API error: %s, body: %s", resp.Status, string(body))
	}

	var localAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&localAIResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(localAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned")
	}

	return parseLLMResponse(localAIResp.Choices[0].Message.Content), nil
}

// HealthCheck checks LocalAI health
func (l *LocalAIClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("http://%s:%d/ready", l.host, l.port)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("localai health check failed: %s", resp.Status)
	}

	return nil
}

// buildPromptFromMessages builds a prompt from chat messages
func buildPromptFromMessages(messages []Message) string {
	var prompt string
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			prompt += msg.Content + "\n\n"
		case "user":
			prompt += "User: " + msg.Content + "\n"
		case "assistant":
			prompt += "Assistant: " + msg.Content + "\n"
		}
	}
	prompt += "Assistant: "
	return prompt
}

// parseLLMResponse parses LLM JSON response
func parseLLMResponse(rawResponse string) *CompletionResult {
	result := &CompletionResult{
		RawResponse: rawResponse,
		Response:    rawResponse,
		Confidence:  0.5,
		Intent:      "general",
	}

	// Try to parse as JSON
	var parsed struct {
		Response        string  `json:"response"`
		TransferToAgent bool    `json:"transfer_to_agent"`
		Reason          string  `json:"reason"`
		Confidence      float64 `json:"confidence"`
		Intent          string  `json:"intent"`
	}

	if err := json.Unmarshal([]byte(rawResponse), &parsed); err == nil {
		result.Response = parsed.Response
		result.TransferToAgent = parsed.TransferToAgent
		result.Reason = parsed.Reason
		result.Confidence = parsed.Confidence
		result.Intent = parsed.Intent
	} else {
		// Fallback: extract JSON from text if embedded
		// This handles cases where LLM adds markdown or extra text
		start := findJSONStart(rawResponse)
		end := findJSONEnd(rawResponse, start)
		if start >= 0 && end > start {
			if err := json.Unmarshal([]byte(rawResponse[start:end+1]), &parsed); err == nil {
				result.Response = parsed.Response
				result.TransferToAgent = parsed.TransferToAgent
				result.Reason = parsed.Reason
				result.Confidence = parsed.Confidence
				result.Intent = parsed.Intent
			}
		}
	}

	return result
}

// findJSONStart finds the start of a JSON object in text
func findJSONStart(text string) int {
	for i, ch := range text {
		if ch == '{' {
			return i
		}
	}
	return -1
}

// findJSONEnd finds the end of a JSON object in text
func findJSONEnd(text string, start int) int {
	if start < 0 || start >= len(text) {
		return -1
	}

	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// getSystemPrompt returns the default system prompt
func getSystemPrompt() string {
	return `You are a helpful voice assistant for a customer service system.
Your job is to understand the caller's intent and help them or route to a human agent when appropriate.

Respond in a natural, conversational way. Keep responses concise and clear for voice.

IMPORTANT: You must respond with valid JSON in this format:
{
  "response": "Your spoken response to the caller",
  "transfer_to_agent": false,
  "reason": null,
  "confidence": 0.85,
  "intent": "billing_inquiry|technical_support|sales|complaint|refund|emergency|general"
}

Set transfer_to_agent to true if:
- The caller explicitly asks for a human agent
- You detect an emergency situation
- The issue requires human judgment (refunds, complaints, billing disputes)
- Your confidence is below 0.6`
}

// MockClient is a mock LLM client for testing
type MockClient struct {
	mockResponse *CompletionResult
}

// NewMockClient creates a new mock LLM client
func NewMockClient(response *CompletionResult) *MockClient {
	return &MockClient{mockResponse: response}
}

// Complete returns a mock completion
func (m *MockClient) Complete(ctx context.Context, messages []Message) (*CompletionResult, error) {
	if m.mockResponse != nil {
		return m.mockResponse, nil
	}
	return &CompletionResult{
		Response:       "Hello, how can I help you today?",
		TransferToAgent: false,
		Confidence:     0.9,
		Intent:         "general",
	}, nil
}

// HealthCheck always returns nil
func (m *MockClient) HealthCheck(ctx context.Context) error {
	return nil
}
