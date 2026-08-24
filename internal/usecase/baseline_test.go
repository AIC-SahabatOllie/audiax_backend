package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"audiax/internal/apperr"
	"audiax/internal/config"
	"audiax/internal/constants"
	"audiax/internal/entity"
	"audiax/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two bytes per float16 value, so the embedding size CHECK the migration
// declares is satisfied by what these tests store.
func fakeEmbeddings(nWindows, dim int) string {
	return base64.StdEncoding.EncodeToString(make([]byte, nWindows*dim*2))
}

func validAIBaseline() *model.AIBaseline {
	return &model.AIBaseline{
		SchemaVersion:      "1.0",
		MachineLabel:       "Blower Oven 1",
		CreatedAt:          "2026-08-23T09:00:00+00:00",
		ModelFingerprint:   "abc123",
		NWindows:           4,
		EmbeddingShape:     []int{4, 8},
		EmbeddingDtype:     constants.EmbeddingDtypeFloat16,
		EmbeddingsB64:      fakeEmbeddings(4, 8),
		BackendStats:       map[string]model.AIBackendStat{"cosine": {Mu: 0.084, Sigma: 0.021}},
		CalibrationQuality: constants.CalibrationQualityGood,
		Notes:              map[string]any{"embedding_dim": 8},
	}
}

func newTestBaselineUseCase(t *testing.T) (*BaselineUseCase, *fakeMachineRepo, *fakeBaselineRepo, *fakeAIService, *fakeObjectStore) {
	t.Helper()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	machines, baselines := newFakeMachineRepo(), newFakeBaselineRepo()
	ai, store := &fakeAIService{baseline: validAIBaseline()}, newFakeObjectStore()

	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-1", Label: "Blower Oven 1"}

	uc := NewBaselineUseCase(newTestDB(t), log, config.NewValidator(), machines, baselines, ai, store)
	return uc, machines, baselines, ai, store
}

func calibrationRequest() *model.CalibrateMachineRequest {
	return &model.CalibrateMachineRequest{
		UserID:    "user-1",
		MachineID: "machine-1",
		Filename:  "calibration.wav",
		Audio:     []byte("wav-bytes"),
	}
}

func TestCalibrateSendsMachineLabelAndStoresBaseline(t *testing.T) {
	uc, _, baselines, ai, _ := newTestBaselineUseCase(t)

	response, err := uc.Calibrate(context.Background(), calibrationRequest())

	require.NoError(t, err)
	// The AI service has no database: the label it fingerprints has to be sent
	// on every call, from machines.label.
	assert.Equal(t, "Blower Oven 1", ai.gotLabel)
	assert.Equal(t, constants.CalibrationQualityGood, response.CalibrationQuality)
	assert.True(t, response.IsActive)

	require.Len(t, baselines.rows, 1)
	stored := baselines.rows[0]
	assert.Equal(t, "abc123", stored.ModelFingerprint)
	assert.Equal(t, 8, stored.EmbeddingDim)
	// Stored as raw bytes, not base64 (docs/erd.md §5.1).
	assert.Len(t, stored.Embeddings, 4*8*2)

	var stats map[string]model.AIBackendStat
	require.NoError(t, json.Unmarshal(stored.BackendStats, &stats))
	assert.InDelta(t, 0.021, stats["cosine"].Sigma, 0.0001)
}

func TestCalibrateDeactivatesThePreviousBaseline(t *testing.T) {
	uc, _, baselines, _, _ := newTestBaselineUseCase(t)
	baselines.rows = append(baselines.rows, &entity.Baseline{
		ID: "old", MachineID: "machine-1", IsActive: true,
	})

	_, err := uc.Calibrate(context.Background(), calibrationRequest())

	require.NoError(t, err)
	assert.Equal(t, 1, baselines.deactivateCalls)
	assert.False(t, baselines.rows[0].IsActive)
	assert.True(t, baselines.rows[1].IsActive)
}

func TestCalibrateStoresAudioUnderTheBaselineID(t *testing.T) {
	uc, _, baselines, _, store := newTestBaselineUseCase(t)

	_, err := uc.Calibrate(context.Background(), calibrationRequest())

	require.NoError(t, err)
	require.NotNil(t, baselines.rows[0].CalibrationAudioPath)
	path := *baselines.rows[0].CalibrationAudioPath
	assert.True(t, strings.HasPrefix(path, constants.CalibrationAudioPrefix))
	assert.Contains(t, path, baselines.rows[0].ID)
	assert.Equal(t, []byte("wav-bytes"), store.puts[path])
}

// Storage is insurance for re-calibrating after a model upgrade, not part of
// the answer. Losing it must not cost the operator a 120-second recording that
// has already produced a valid baseline.
func TestCalibrateSucceedsEvenIfAudioUploadFails(t *testing.T) {
	uc, _, baselines, _, store := newTestBaselineUseCase(t)
	store.err = errors.New("bucket unreachable")

	response, err := uc.Calibrate(context.Background(), calibrationRequest())

	require.NoError(t, err)
	assert.True(t, response.IsActive)
	assert.Nil(t, baselines.rows[0].CalibrationAudioPath)
}

// The quality gate's message is the only text telling the operator what to do
// differently, so it has to survive the whole way out.
func TestCalibrateRejectionFromAIReachesTheCaller(t *testing.T) {
	uc, _, _, ai, _ := newTestBaselineUseCase(t)
	ai.err = &apperr.RejectedError{Reason: "Audio kalibrasi tidak lolos quality gate."}

	_, err := uc.Calibrate(context.Background(), calibrationRequest())

	var rejected *apperr.RejectedError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, "Audio kalibrasi tidak lolos quality gate.", rejected.Reason)
}

func TestCalibrateOnSomeoneElsesMachineIsNotFound(t *testing.T) {
	uc, machines, _, ai, _ := newTestBaselineUseCase(t)
	machines.byID["machine-2"] = &entity.Machine{ID: "machine-2", UserID: "user-2", Label: "Theirs"}

	request := calibrationRequest()
	request.MachineID = "machine-2"
	_, err := uc.Calibrate(context.Background(), request)

	assert.ErrorIs(t, err, apperr.ErrNotFound)
	// Ownership is settled before any audio leaves this process.
	assert.Empty(t, ai.gotLabel)
}

func TestCalibrateRejectsUnexpectedEmbeddingDtype(t *testing.T) {
	uc, _, baselines, ai, _ := newTestBaselineUseCase(t)
	ai.baseline.EmbeddingDtype = "float32"

	_, err := uc.Calibrate(context.Background(), calibrationRequest())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "float32")
	// A rejected payload must not have disturbed the machine's active baseline.
	assert.Empty(t, baselines.rows)
	assert.Zero(t, baselines.deactivateCalls)
}

func TestListBaselinesIsScopedToTheOwner(t *testing.T) {
	uc, machines, baselines, _, _ := newTestBaselineUseCase(t)
	machines.byID["machine-2"] = &entity.Machine{ID: "machine-2", UserID: "user-2", Label: "Theirs"}
	baselines.rows = append(baselines.rows,
		&entity.Baseline{ID: "b1", MachineID: "machine-1", IsActive: true},
		&entity.Baseline{ID: "b2", MachineID: "machine-2"},
	)

	mine, err := uc.List(context.Background(), "user-1", "machine-1")
	require.NoError(t, err)
	require.Len(t, mine, 1)
	assert.Equal(t, "b1", mine[0].ID)

	_, err = uc.List(context.Background(), "user-1", "machine-2")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}
