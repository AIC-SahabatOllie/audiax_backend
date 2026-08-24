package converter

import (
	"audiax/internal/entity"
	"audiax/internal/model"
)

func BaselineToResponse(baseline *entity.Baseline) *model.BaselineResponse {
	return &model.BaselineResponse{
		ID:                 baseline.ID,
		MachineID:          baseline.MachineID,
		ModelFingerprint:   baseline.ModelFingerprint,
		NWindows:           baseline.NWindows,
		CalibrationQuality: baseline.CalibrationQuality,
		IsActive:           baseline.IsActive,
		CalibratedAt:       baseline.CalibratedAt,
	}
}

func BaselinesToResponses(baselines []entity.Baseline) []model.BaselineResponse {
	out := make([]model.BaselineResponse, 0, len(baselines))
	for i := range baselines {
		out = append(out, *BaselineToResponse(&baselines[i]))
	}
	return out
}
