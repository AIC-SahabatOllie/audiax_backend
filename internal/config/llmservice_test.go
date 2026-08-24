package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"audiax/internal/apperr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLLMServiceCompleteSendsTheFixedDeterministicOptions(t *testing.T) {
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"jawaban\":\"ok\"}"}}`))
	}))
	defer server.Close()

	out, err := NewLLMService(server.URL, "audiax-advisor", time.Second).
		Complete(t.Context(), "FAKTA:\n  status: WARNING")

	require.NoError(t, err)
	assert.Equal(t, `{"jawaban":"ok"}`, out)

	assert.Equal(t, "audiax-advisor", gotBody["model"])
	assert.Equal(t, false, gotBody["stream"])
	assert.Equal(t, "json", gotBody["format"])
	options := gotBody["options"].(map[string]any)
	assert.Equal(t, float64(0), options["temperature"])
	assert.Equal(t, float64(1), options["top_k"])
	assert.Equal(t, float64(1), options["top_p"])
	assert.Equal(t, float64(42), options["seed"])

	messages := gotBody["messages"].([]any)
	require.Len(t, messages, 1)
	firstMessage := messages[0].(map[string]any)
	assert.Equal(t, "user", firstMessage["role"])
	assert.Equal(t, "FAKTA:\n  status: WARNING", firstMessage["content"])
}

func TestLLMServiceCompleteWrapsNon200AsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("model not loaded"))
	}))
	defer server.Close()

	_, err := NewLLMService(server.URL, "audiax-advisor", time.Second).Complete(t.Context(), "prompt")

	assert.ErrorIs(t, err, apperr.ErrUnavailable)
}

func TestLLMServiceCompleteWrapsTransportFailureAsUnavailable(t *testing.T) {
	_, err := NewLLMService("http://127.0.0.1:1", "audiax-advisor", 50*time.Millisecond).
		Complete(t.Context(), "prompt")

	assert.ErrorIs(t, err, apperr.ErrUnavailable)
}
