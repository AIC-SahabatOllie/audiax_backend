package model

import "time"

type InspectionResponse struct {
	ID                string   `json:"id"`
	MachineID         string   `json:"machine_id"`
	BaselineID        string   `json:"baseline_id"`
	Status            string   `json:"status"`
	ZScore            *float64 `json:"z_score"`
	HealthScore       *float64 `json:"health_score"`
	DominantIndicator *string  `json:"dominant_indicator"`
	Reason            *string  `json:"reason"`
	// Disclaimer is always populated. A client must display it: this system is
	// a triage aid, not a diagnosis (docs/prd.md §3).
	Disclaimer  string    `json:"disclaimer"`
	InspectedAt time.Time `json:"inspected_at"`
}

// InspectMachineRequest is assembled by the controller from a multipart upload.
type InspectMachineRequest struct {
	UserID    string `validate:"required"`
	MachineID string `validate:"required"`
	Filename  string `validate:"required"`
	Audio     []byte `validate:"required,gt=0"`
}
