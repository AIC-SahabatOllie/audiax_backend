package model

// The structs below mirror the JSON the AI service (../audiax_model) speaks.
// They are wire DTOs, not entities: that service owns their shape, and
// audiax_model/ai/calibration.py is the source of truth for the baseline.

type AIBackendStat struct {
	Mu    float64 `json:"mu"`
	Sigma float64 `json:"sigma"`
}

// AIBaseline mirrors MachineBaseline.to_json_dict().
type AIBaseline struct {
	SchemaVersion      string                   `json:"schema_version"`
	MachineLabel       string                   `json:"machine_label"`
	CreatedAt          string                   `json:"created_at"`
	ModelFingerprint   string                   `json:"model_fingerprint"`
	NWindows           int                      `json:"n_windows"`
	EmbeddingShape     []int                    `json:"embedding_shape"`
	EmbeddingDtype     string                   `json:"embedding_dtype"`
	EmbeddingsB64      string                   `json:"embeddings_b64"`
	BackendStats       map[string]AIBackendStat `json:"backend_stats"`
	CalibrationQuality string                   `json:"calibration_quality"`
	Notes              map[string]any           `json:"notes"`
}

// AIHealthCard mirrors HealthCard.to_dict().
//
// ZScore, HealthScore, DominantIndicator and Reason are pointers because the AI
// service returns null for each of them on legitimate paths: a non-finite z on
// KALIBRASI_KURANG, no dominant indicator on NORMAL, no reason otherwise.
//
// HealthScore and Reason are additionally absent from the response
// audiax_model/service/main.py builds today: it assembles the body by hand and
// drops both, even though decision.py documents HealthCard.to_dict() as the
// shape /v1/inspect returns. Pointers keep this client correct either way, and
// both fields stay null until that service is fixed.
type AIHealthCard struct {
	Status             string   `json:"status"`
	ZScore             *float64 `json:"z_score"`
	HealthScore        *float64 `json:"health_score"`
	CalibrationQuality string   `json:"calibration_quality"`
	DominantIndicator  *string  `json:"dominant_indicator"`
	Disclaimer         string   `json:"disclaimer"`
	Reason             *string  `json:"reason"`
}
