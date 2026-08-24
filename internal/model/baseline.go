package model

import "time"

// BaselineResponse is what a client is told about a calibration. Embeddings,
// backend stats and notes are deliberately absent: they are the AI service's
// working material, and no client has a use for 60-178 KB of memory bank.
type BaselineResponse struct {
	ID                 string    `json:"id"`
	MachineID          string    `json:"machine_id"`
	ModelFingerprint   string    `json:"model_fingerprint"`
	NWindows           int       `json:"n_windows"`
	CalibrationQuality string    `json:"calibration_quality"`
	IsActive           bool      `json:"is_active"`
	CalibratedAt       time.Time `json:"calibrated_at"`
}

// CalibrateMachineRequest is assembled by the controller from a multipart
// upload, not parsed from a JSON body.
type CalibrateMachineRequest struct {
	UserID    string `validate:"required"`
	MachineID string `validate:"required"`
	Filename  string `validate:"required"`
	Audio     []byte `validate:"required,gt=0"`
}
