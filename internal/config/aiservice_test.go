package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"audiax/internal/apperr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAIServiceCalibrateParsesBaseline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/calibrate", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(1<<20))
		assert.Equal(t, "Blower Oven 1", r.FormValue("machine_label"))

		file, header, err := r.FormFile("audio")
		require.NoError(t, err)
		defer file.Close()
		assert.Equal(t, "calibration.wav", header.Filename)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"schema_version":"1.0","machine_label":"Blower Oven 1",
			"created_at":"2026-08-23T09:00:00+00:00","model_fingerprint":"abc123",
			"n_windows":119,"embedding_shape":[119,256],"embedding_dtype":"float16",
			"embeddings_b64":"AAA=","backend_stats":{"cosine":{"mu":0.084,"sigma":0.021}},
			"calibration_quality":"baik","notes":{"embedding_dim":256}}`))
	}))
	defer server.Close()

	baseline, err := NewAIService(server.URL, time.Second).
		Calibrate(context.Background(), []byte("fake-wav"), "calibration.wav", "Blower Oven 1")

	require.NoError(t, err)
	assert.Equal(t, "abc123", baseline.ModelFingerprint)
	assert.Equal(t, 119, baseline.NWindows)
	assert.Equal(t, []int{119, 256}, baseline.EmbeddingShape)
	assert.Equal(t, "baik", baseline.CalibrationQuality)
	assert.InDelta(t, 0.021, baseline.BackendStats["cosine"].Sigma, 0.0001)
}

// The quality gate's message is the only text that tells the operator what to do
// differently. Collapsing it into a bare status would leave them guessing.
func TestAIServiceTurns400IntoRejectionWithReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"Audio kalibrasi tidak lolos quality gate (terlalu senyap)."}`))
	}))
	defer server.Close()

	_, err := NewAIService(server.URL, time.Second).
		Calibrate(context.Background(), []byte("fake-wav"), "calibration.wav", "Blower Oven 1")

	var rejected *apperr.RejectedError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, "Audio kalibrasi tidak lolos quality gate (terlalu senyap).", rejected.Reason)
}

// /healthz returns 503 until the BEATs checkpoint finishes loading, and the
// inference endpoints fail the same way. That is the dependency's problem, not
// the caller's request.
func TestAIServiceTurns503IntoUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"detail":"Model belum siap."}`))
	}))
	defer server.Close()

	_, err := NewAIService(server.URL, time.Second).
		Inspect(context.Background(), []byte("fake-wav"), "test.wav", []byte(`{}`))

	assert.ErrorIs(t, err, apperr.ErrUnavailable)
}

func TestAIServiceInspectSendsBaselineAndParsesCard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/inspect", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(1<<20))
		assert.JSONEq(t, `{"schema_version":"1.0"}`, r.FormValue("baseline_json"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"WARNING","z_score":4.2,
			"calibration_quality":"baik","dominant_indicator":"kurtosis",
			"disclaimer":"Alat bantu triase, bukan diagnosis mengikat -- tetap perlu inspeksi teknisi."}`))
	}))
	defer server.Close()

	card, err := NewAIService(server.URL, time.Second).
		Inspect(context.Background(), []byte("fake-wav"), "test.wav", []byte(`{"schema_version":"1.0"}`))

	require.NoError(t, err)
	assert.Equal(t, "WARNING", card.Status)
	require.NotNil(t, card.ZScore)
	assert.InDelta(t, 4.2, *card.ZScore, 0.0001)
	require.NotNil(t, card.DominantIndicator)
	assert.Equal(t, "kurtosis", *card.DominantIndicator)
	assert.NotEmpty(t, card.Disclaimer)
	// Absent from the body service/main.py builds today; must decode as nil
	// rather than a misleading zero.
	assert.Nil(t, card.HealthScore)
	assert.Nil(t, card.Reason)
}

// A KALIBRASI_KURANG card is a normal 200 response with a null score, not an
// error. Decoding it must not fail.
func TestAIServiceInspectAcceptsNullScores(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"KALIBRASI_KURANG","z_score":null,"health_score":null,
			"calibration_quality":"rendah","dominant_indicator":null,
			"disclaimer":"d","reason":"Rekaman kalibrasi terlalu pendek."}`))
	}))
	defer server.Close()

	card, err := NewAIService(server.URL, time.Second).
		Inspect(context.Background(), []byte("fake-wav"), "test.wav", []byte(`{}`))

	require.NoError(t, err)
	assert.Equal(t, "KALIBRASI_KURANG", card.Status)
	assert.Nil(t, card.ZScore)
	require.NotNil(t, card.Reason)
	assert.Equal(t, "Rekaman kalibrasi terlalu pendek.", *card.Reason)
}

func TestAIServiceUnreachableIsUnavailable(t *testing.T) {
	// Port 0 on loopback is never listening.
	_, err := NewAIService("http://127.0.0.1:0", 200*time.Millisecond).
		Inspect(context.Background(), []byte("fake-wav"), "test.wav", []byte(`{}`))

	assert.ErrorIs(t, err, apperr.ErrUnavailable)
}
