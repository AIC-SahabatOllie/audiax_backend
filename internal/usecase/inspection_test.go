package usecase

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"audiax/internal/apperr"
	"audiax/internal/config"
	"audiax/internal/constants"
	"audiax/internal/entity"
	"audiax/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
)

func float64Ptr(v float64) *float64 { return &v }
func stringPtr(v string) *string    { return &v }

func newTestInspectionUseCase(t *testing.T) (*InspectionUseCase, *fakeMachineRepo, *fakeBaselineRepo, *fakeInspectionRepo, *fakeAIService, *fakeObjectStore) {
	t.Helper()

	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	require.NoError(t, err)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	machines, baselines := newFakeMachineRepo(), newFakeBaselineRepo()
	inspections, store := newFakeInspectionRepo(), newFakeObjectStore()

	ai := &fakeAIService{card: &model.AIHealthCard{
		Status:             constants.StatusNormal,
		ZScore:             float64Ptr(1.2),
		HealthScore:        float64Ptr(80),
		CalibrationQuality: constants.CalibrationQualityGood,
		Disclaimer:         constants.TriageDisclaimer,
	}}

	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-1", Label: "Blower Oven 1"}
	baselines.rows = append(baselines.rows, &entity.Baseline{
		ID: "baseline-1", MachineID: "machine-1", IsActive: true,
		SchemaVersion: "1.0", ModelFingerprint: "abc123",
		NWindows: 4, EmbeddingDim: 8, EmbeddingDtype: constants.EmbeddingDtypeFloat16,
		Embeddings:         make([]byte, 4*8*2),
		BackendStats:       []byte(`{"cosine":{"mu":0.084,"sigma":0.021}}`),
		Notes:              []byte(`{"embedding_dim":8}`),
		CalibrationQuality: constants.CalibrationQualityGood,
	})

	uc := NewInspectionUseCase(db, log, config.NewValidator(),
		machines, baselines, inspections, ai, store)
	return uc, machines, baselines, inspections, ai, store
}

func TestInspectRebuildsTheBaselinePayloadForTheAIService(t *testing.T) {
	uc, machines, _, _, ai, _ := newTestInspectionUseCase(t)

	_, err := uc.Inspect(context.Background(), &model.InspectMachineRequest{
		UserID: "user-1", MachineID: "machine-1", Filename: "t.wav", Audio: []byte("wav"),
	})
	require.NoError(t, err)

	var sent model.AIBaseline
	require.NoError(t, json.Unmarshal(ai.gotBaseline, &sent))
	assert.Equal(t, "1.0", sent.SchemaVersion)
	assert.Equal(t, "abc123", sent.ModelFingerprint)
	assert.Equal(t, []int{4, 8}, sent.EmbeddingShape)
	assert.Equal(t, constants.EmbeddingDtypeFloat16, sent.EmbeddingDtype)
	assert.NotEmpty(t, sent.EmbeddingsB64)
	assert.InDelta(t, 0.021, sent.BackendStats["cosine"].Sigma, 0.0001)
	// The label comes from the machine, not from a stored copy.
	assert.Equal(t, machines.byID["machine-1"].Label, sent.MachineLabel)
}

func TestInspectStoresTheCardAndAlwaysReturnsADisclaimer(t *testing.T) {
	uc, _, _, inspections, _, _ := newTestInspectionUseCase(t)

	response, err := uc.Inspect(context.Background(), &model.InspectMachineRequest{
		UserID: "user-1", MachineID: "machine-1", Filename: "t.wav", Audio: []byte("wav"),
	})

	require.NoError(t, err)
	assert.Equal(t, constants.StatusNormal, response.Status)
	assert.Equal(t, constants.TriageDisclaimer, response.Disclaimer)
	require.Len(t, inspections.rows, 1)
	assert.Equal(t, "baseline-1", inspections.rows[0].BaselineID)
}

// A NORMAL result is not evidence worth keeping, and storing every clip costs
// money and privacy exposure for nothing (docs/prd.md FR10).
func TestInspectDiscardsAudioWhenNormal(t *testing.T) {
	uc, _, _, inspections, _, store := newTestInspectionUseCase(t)

	_, err := uc.Inspect(context.Background(), &model.InspectMachineRequest{
		UserID: "user-1", MachineID: "machine-1", Filename: "t.wav", Audio: []byte("wav"),
	})

	require.NoError(t, err)
	assert.Empty(t, store.puts)
	assert.Nil(t, inspections.rows[0].AudioPath)
}

func TestInspectKeepsAudioWhenAnomalous(t *testing.T) {
	uc, _, _, inspections, ai, store := newTestInspectionUseCase(t)
	ai.card.Status = constants.StatusCritical
	ai.card.ZScore = float64Ptr(7.4)
	ai.card.DominantIndicator = stringPtr("kurtosis")

	_, err := uc.Inspect(context.Background(), &model.InspectMachineRequest{
		UserID: "user-1", MachineID: "machine-1", Filename: "t.wav", Audio: []byte("wav-bytes"),
	})

	require.NoError(t, err)
	require.NotNil(t, inspections.rows[0].AudioPath)
	assert.Equal(t, []byte("wav-bytes"), store.puts[*inspections.rows[0].AudioPath])
}

// KALIBRASI_KURANG is a normal outcome, not a failure: it is stored, returned
// with 200, and its reason tells the operator what to do.
func TestInspectStoresUncalibratedResultWithNullScores(t *testing.T) {
	uc, _, _, inspections, ai, _ := newTestInspectionUseCase(t)
	ai.card.Status = constants.StatusUncalibrated
	ai.card.ZScore = nil
	ai.card.HealthScore = nil
	ai.card.Reason = stringPtr("Rekaman kalibrasi terlalu pendek. Rekam ulang lebih lama.")

	response, err := uc.Inspect(context.Background(), &model.InspectMachineRequest{
		UserID: "user-1", MachineID: "machine-1", Filename: "t.wav", Audio: []byte("wav"),
	})

	require.NoError(t, err)
	assert.Equal(t, constants.StatusUncalibrated, response.Status)
	assert.Nil(t, response.ZScore)
	require.NotNil(t, response.Reason)
	require.Len(t, inspections.rows, 1)
	assert.Nil(t, inspections.rows[0].ZScore)
}

func TestInspectWithoutAnActiveBaselineIsRejectedWithGuidance(t *testing.T) {
	uc, _, baselines, _, ai, _ := newTestInspectionUseCase(t)
	baselines.rows = nil

	_, err := uc.Inspect(context.Background(), &model.InspectMachineRequest{
		UserID: "user-1", MachineID: "machine-1", Filename: "t.wav", Audio: []byte("wav"),
	})

	var rejected *apperr.RejectedError
	require.ErrorAs(t, err, &rejected)
	assert.Contains(t, rejected.Reason, "kalibrasi")
	assert.Zero(t, ai.inspectCalls, "the AI service must not be called without a baseline")
}

func TestInspectOnSomeoneElsesMachineIsNotFound(t *testing.T) {
	uc, machines, _, _, _, _ := newTestInspectionUseCase(t)
	machines.byID["machine-2"] = &entity.Machine{ID: "machine-2", UserID: "user-2", Label: "Theirs"}

	_, err := uc.Inspect(context.Background(), &model.InspectMachineRequest{
		UserID: "user-1", MachineID: "machine-2", Filename: "t.wav", Audio: []byte("wav"),
	})

	assert.ErrorIs(t, err, apperr.ErrNotFound)
}
