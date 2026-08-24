package repository

import (
	"audiax/internal/entity"

	"gorm.io/gorm"
)

type BaselineRepository struct {
	Repository[entity.Baseline]
}

func NewBaselineRepository() *BaselineRepository { return &BaselineRepository{} }

// FindActiveForMachine loads the whole row, embeddings included: its only caller
// is the inspection path, which needs them to rebuild the AI payload.
func (r *BaselineRepository) FindActiveForMachine(db *gorm.DB, baseline *entity.Baseline, machineID string) error {
	return db.Where("machine_id = ? and is_active", machineID).Take(baseline).Error
}

// DeactivateAllForMachine clears the active flag before a new baseline claims
// it. It must run inside the caller's transaction, so the partial unique index
// is never even briefly violated.
func (r *BaselineRepository) DeactivateAllForMachine(db *gorm.DB, machineID string) error {
	return db.Model(&entity.Baseline{}).
		Where("machine_id = ? and is_active", machineID).
		Update("is_active", false).Error
}

// UpdateAudioPath records where the calibration recording was stored. It is
// separate from Create because the upload happens after the row is committed: a
// storage outage must not cost the operator a 120-second recording that already
// produced a valid baseline.
func (r *BaselineRepository) UpdateAudioPath(db *gorm.DB, baselineID, path string) error {
	return db.Model(&entity.Baseline{}).
		Where("id = ?", baselineID).
		Update("calibration_audio_path", path).Error
}

// ListForMachine omits embeddings: a history list has no use for 60-178 KB per
// row, and selecting them would make the endpoint pathologically slow.
func (r *BaselineRepository) ListForMachine(db *gorm.DB, baselines *[]entity.Baseline, machineID string) error {
	return db.Select("id", "machine_id", "schema_version", "model_fingerprint",
		"n_windows", "embedding_dim", "embedding_dtype", "calibration_quality",
		"calibration_audio_path", "is_active", "calibrated_at", "created_at").
		Where("machine_id = ?", machineID).
		Order("calibrated_at desc").
		Find(baselines).Error
}
