package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"audiax/internal/apperr"
	"audiax/internal/constants"
)

// LLMService is the HTTP client for the Ollama container that serves the
// fine-tuned advisory model (DESIGN.md §3.6). It implements
// advisory.LLMProvider; that interface lives in internal/advisory so the
// pure package never imports net/http, following the same split as
// AIService (declared in internal/usecase, implemented here).
type LLMService struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewLLMService(baseURL, model string, timeout time.Duration) *LLMService {
	return &LLMService{baseURL: baseURL, model: model, client: &http.Client{Timeout: timeout}}
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   string          `json:"format"`
	Options  ollamaOptions   `json:"options"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaOptions is fixed, not configurable: temperature 0 / top_k 1 / top_p 1
// / a pinned seed is what makes DESIGN.md §6's determinism metric (identical
// output on two runs) meaningful at all.
type ollamaOptions struct {
	Temperature float64 `json:"temperature"`
	TopK        int     `json:"top_k"`
	TopP        float64 `json:"top_p"`
	Seed        int     `json:"seed"`
	NumPredict  int     `json:"num_predict"`
}

type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
}

// Complete sends the already-rendered prompt to Ollama's chat endpoint and
// returns the raw completion text. It does not parse or validate that text
// as JSON: guard.go owns that, and it needs the raw string to report a
// useful schema error when the model misbehaves.
func (s *LLMService) Complete(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(ollamaChatRequest{
		Model:    s.model,
		Messages: []ollamaMessage{{Role: "user", Content: prompt}},
		Stream:   false,
		Format:   "json",
		Options: ollamaOptions{
			Temperature: 0, TopK: 1, TopP: 1, Seed: 42,
			NumPredict: constants.AdvisoryNumPredict,
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.baseURL+constants.OllamaChatPath, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		// A transport failure (including our own timeout) is the
		// dependency's problem, not the caller's -- the use case treats this
		// exactly like a guard failure and falls back to StaticReply.
		return "", fmt.Errorf("%w: call ollama: %v", apperr.ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return "", fmt.Errorf("%w: ollama returned %d: %s", apperr.ErrUnavailable, resp.StatusCode, string(raw))
	}

	var out ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	return out.Message.Content, nil
}
