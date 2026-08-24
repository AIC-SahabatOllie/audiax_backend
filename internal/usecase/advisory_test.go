package usecase

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"audiax/internal/advisory"
	"audiax/internal/apperr"
	"audiax/internal/config"
	"audiax/internal/constants"
	"audiax/internal/entity"
	"audiax/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAdvisoryUseCase(t *testing.T) (*AdvisoryUseCase, *fakeMachineRepo, *fakeInspectionRepo, *fakeBaselineRepo, *fakeLLMProvider) {
	t.Helper()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	machines, inspections, baselines := newFakeMachineRepo(), newFakeInspectionRepo(), newFakeBaselineRepo()
	llm := &fakeLLMProvider{}

	uc := NewAdvisoryUseCase(newTestDB(t), log, config.NewValidator(),
		machines, baselines, inspections, llm, time.Second)
	return uc, machines, inspections, baselines, llm
}

func warningInspection() *entity.Inspection {
	z := 3.4
	indicator := "crest_factor"
	return &entity.Inspection{
		ID:                "inspection-1",
		MachineID:         "machine-1",
		BaselineID:        "baseline-1",
		Status:            constants.StatusWarning,
		ZScore:            &z,
		DominantIndicator: &indicator,
		InspectedAt:       time.Now().UTC(),
	}
}

func advisoryRequest() *model.AdvisoryMessageRequest {
	return &model.AdvisoryMessageRequest{
		UserID:       "user-1",
		MachineID:    "machine-1",
		InspectionID: "inspection-1",
		UserMessage:  "sabuknya kenceng kok",
		Context: model.AdvisoryContext{
			DriveType:   "belt",
			Recency:     "1-6bln",
			MachineAge:  "3-5th",
			HoursPerDay: ">8",
			HasBackup:   false,
			LoadState:   "bermuatan",
		},
	}
}

func TestAdvisorySendReturnsNotFoundWhenMachineNotOwnedByUser(t *testing.T) {
	uc, machines, _, _, _ := newTestAdvisoryUseCase(t)
	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "someone-else"}

	_, err := uc.Send(t.Context(), advisoryRequest())

	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestAdvisorySendReturnsNotFoundWhenInspectionNotOnThatMachine(t *testing.T) {
	uc, machines, _, _, _ := newTestAdvisoryUseCase(t)
	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-1"}
	// No inspection seeded at all.

	_, err := uc.Send(t.Context(), advisoryRequest())

	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestAdvisorySendFailsValidationWithoutUserMessage(t *testing.T) {
	uc, machines, inspections, _, _ := newTestAdvisoryUseCase(t)
	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-1"}
	inspections.rows = append(inspections.rows, warningInspection())

	request := advisoryRequest()
	request.UserMessage = ""

	_, err := uc.Send(t.Context(), request)

	var validationErr *apperr.ValidationError
	assert.ErrorAs(t, err, &validationErr)
}

func TestAdvisorySendFallsBackToStaticWhenLLMUnavailable(t *testing.T) {
	uc, machines, inspections, baselines, llm := newTestAdvisoryUseCase(t)
	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-1"}
	inspections.rows = append(inspections.rows, warningInspection())
	baselines.rows = append(baselines.rows, &entity.Baseline{ID: "baseline-1", MachineID: "machine-1", CalibrationQuality: constants.CalibrationQualityGood})
	llm.err = apperr.ErrUnavailable

	response, err := uc.Send(t.Context(), advisoryRequest())

	require.NoError(t, err)
	assert.Equal(t, constants.AdvisorySourceFallbackStatic, response.Source)
	assert.NotEmpty(t, response.Reply)
	assert.NotEmpty(t, response.NextStep)
	assert.Equal(t, constants.TriageDisclaimer, response.Disclaimer)
}

func TestAdvisorySendUsesLLMReplyWhenGuardPasses(t *testing.T) {
	uc, machines, inspections, baselines, llm := newTestAdvisoryUseCase(t)
	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-1"}
	inspections.rows = append(inspections.rows, warningInspection())
	baselines.rows = append(baselines.rows, &entity.Baseline{ID: "baseline-1", MachineID: "machine-1", CalibrationQuality: constants.CalibrationQualityGood})

	cell, err := advisory.Lookup(constants.StatusWarning, "crest_factor", "belt", "1-6bln")
	require.NoError(t, err)
	require.NotEmpty(t, cell.SafetyGate)

	llm.completion = `{"jawaban":"Statusnya WARNING. ` + cell.SafetyGate + ` Baru periksa ketegangan sabuk.","langkah_berikutnya":"Periksa ketegangan sabuk sesuai checklist.","perlu_teknisi":false,"eskalasi":false}`

	response, err := uc.Send(t.Context(), advisoryRequest())

	require.NoError(t, err)
	assert.Equal(t, constants.AdvisorySourceLLM, response.Source)
	assert.Contains(t, response.Reply, "Statusnya WARNING")
	assert.False(t, response.NeedsTechnician)
	assert.False(t, response.Escalated)
}

func TestAdvisorySendFallsBackWhenGuardRejectsLLMOutput(t *testing.T) {
	uc, machines, inspections, baselines, llm := newTestAdvisoryUseCase(t)
	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-1"}
	inspections.rows = append(inspections.rows, warningInspection())
	baselines.rows = append(baselines.rows, &entity.Baseline{ID: "baseline-1", MachineID: "machine-1", CalibrationQuality: constants.CalibrationQualityGood})
	llm.completion = `{"jawaban":"Ini kemungkinan bearing aus.","langkah_berikutnya":"Ganti bearing.","perlu_teknisi":true,"eskalasi":false}`

	response, err := uc.Send(t.Context(), advisoryRequest())

	require.NoError(t, err)
	assert.Equal(t, constants.AdvisorySourceFallbackStatic, response.Source)
}

func TestAdvisorySendOmitsCalibrationQualityFromPromptWhenBaselineJoinFails(t *testing.T) {
	uc, machines, inspections, _, llm := newTestAdvisoryUseCase(t)
	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-1"}
	inspections.rows = append(inspections.rows, warningInspection())
	// No baseline seeded: the join fails.
	llm.err = apperr.ErrUnavailable

	_, err := uc.Send(t.Context(), advisoryRequest())

	require.NoError(t, err)
	assert.NotContains(t, llm.gotPrompt, "kualitas_kalibrasi:")
}

func TestAdvisorySendForcesCriticalPathOnDangerKeywordWithoutCallingLLM(t *testing.T) {
	uc, machines, inspections, baselines, llm := newTestAdvisoryUseCase(t)
	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-1"}
	// The inspection itself is NORMAL: the acoustic model saw nothing wrong.
	normal := warningInspection()
	normal.Status = constants.StatusNormal
	normal.DominantIndicator = nil
	inspections.rows = append(inspections.rows, normal)
	baselines.rows = append(baselines.rows, &entity.Baseline{ID: "baseline-1", MachineID: "machine-1", CalibrationQuality: constants.CalibrationQualityGood})

	request := advisoryRequest()
	request.UserMessage = "kok tiba-tiba ada bau gosong ya"

	response, err := uc.Send(t.Context(), request)

	require.NoError(t, err)
	assert.True(t, response.NeedsTechnician)
	assert.True(t, response.Escalated)
	assert.Equal(t, 0, llm.calls, "a danger keyword must short-circuit straight to the safety fallback, never involve the LLM")
}
