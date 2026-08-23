package config

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"audiax/internal/apperr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObjectStorePutSendsAuthenticatedUpload(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotType string
	var gotBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Key":"audiax-audio/calibrations/x.wav"}`))
	}))
	defer server.Close()

	store := NewObjectStore(server.URL, "service-key", "audiax-audio", time.Second)
	err := store.Put(context.Background(), "calibrations/m1/b1.wav", []byte("wav-bytes"), "audio/wav")

	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/storage/v1/object/audiax-audio/calibrations/m1/b1.wav", gotPath)
	assert.Equal(t, "Bearer service-key", gotAuth)
	assert.Equal(t, "audio/wav", gotType)
	assert.Equal(t, []byte("wav-bytes"), gotBody)
}

func TestObjectStorePutReportsUnavailableOnServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	store := NewObjectStore(server.URL, "service-key", "audiax-audio", time.Second)
	err := store.Put(context.Background(), "calibrations/x.wav", []byte("wav-bytes"), "audio/wav")

	assert.ErrorIs(t, err, apperr.ErrUnavailable)
}

// A duplicate key comes back as 409. It is still a storage failure the caller
// cannot act on, so it must not masquerade as success.
func TestObjectStorePutReportsConflictAsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"Duplicate","message":"The resource already exists"}`))
	}))
	defer server.Close()

	store := NewObjectStore(server.URL, "service-key", "audiax-audio", time.Second)
	err := store.Put(context.Background(), "calibrations/x.wav", []byte("wav-bytes"), "audio/wav")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "409")
}

func TestObjectStoreUnreachableIsUnavailable(t *testing.T) {
	store := NewObjectStore("http://127.0.0.1:0", "service-key", "audiax-audio", 200*time.Millisecond)
	err := store.Put(context.Background(), "calibrations/x.wav", []byte("wav-bytes"), "audio/wav")

	assert.ErrorIs(t, err, apperr.ErrUnavailable)
}
