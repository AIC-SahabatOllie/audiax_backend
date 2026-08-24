package converter

import (
	"audiax/internal/entity"
	"audiax/internal/model"
)

// InspectionToResponse takes the disclaimer as a parameter rather than reading a
// constant, so a live inspection can echo the exact text the AI service returned
// while history rows fall back to constants.TriageDisclaimer.
func InspectionToResponse(inspection *entity.Inspection, disclaimer string) *model.InspectionResponse {
	return &model.InspectionResponse{
		ID:                inspection.ID,
		MachineID:         inspection.MachineID,
		BaselineID:        inspection.BaselineID,
		Status:            inspection.Status,
		ZScore:            inspection.ZScore,
		HealthScore:       inspection.HealthScore,
		DominantIndicator: inspection.DominantIndicator,
		Reason:            inspection.Reason,
		Disclaimer:        disclaimer,
		InspectedAt:       inspection.InspectedAt,
	}
}

func InspectionsToResponses(inspections []entity.Inspection, disclaimer string) []model.InspectionResponse {
	out := make([]model.InspectionResponse, 0, len(inspections))
	for i := range inspections {
		out = append(out, *InspectionToResponse(&inspections[i], disclaimer))
	}
	return out
}
