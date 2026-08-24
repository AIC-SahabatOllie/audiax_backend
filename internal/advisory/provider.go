package advisory

import "context"

// LLMProvider is implemented outside this package, in
// internal/config/llmservice.go, following the same pattern as the existing
// AIService HTTP client. Declaring the interface here (and not the concrete
// Ollama client) is what keeps this package free of an HTTP dependency.
type LLMProvider interface {
	Complete(ctx context.Context, prompt string) (string, error)
}
