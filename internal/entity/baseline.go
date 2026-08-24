package entity

import "time"

// Baseline is one calibration of one machine: its memory bank plus the score
// scale that belongs to that unit alone. Baselines are never deleted, only
// deactivated -- a z-score means nothing without the baseline that produced it
// (docs/erd.md §5.4).
//
// Embeddings is large (60-178 KB). Never load it in a read path that does not
// need it; ListForMachine deliberately leaves it out (docs/erd.md §5.1).
type Baseline struct {
	ID                   string    `gorm:"column:id;primaryKey"`
	MachineID            string    `gorm:"column:machine_id"`
	SchemaVersion        string    `gorm:"column:schema_version"`
	ModelFingerprint     string    `gorm:"column:model_fingerprint"`
	NWindows             int       `gorm:"column:n_windows"`
	EmbeddingDim         int       `gorm:"column:embedding_dim"`
	EmbeddingDtype       string    `gorm:"column:embedding_dtype"`
	Embeddings           []byte    `gorm:"column:embeddings"`
	BackendStats         []byte    `gorm:"column:backend_stats;type:jsonb"`
	Notes                []byte    `gorm:"column:notes;type:jsonb"`
	CalibrationQuality   string    `gorm:"column:calibration_quality"`
	CalibrationAudioPath *string   `gorm:"column:calibration_audio_path"`
	IsActive             bool      `gorm:"column:is_active"`
	CalibratedAt         time.Time `gorm:"column:calibrated_at"`
	CreatedAt            time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Baseline) TableName() string { return "baselines" }
