package entity

import "time"

// Inspection is one health card. ZScore and HealthScore are nullable because
// HealthCard.to_dict() emits null for a non-finite z, which is exactly what a
// KALIBRASI_KURANG result carries -- the most common outcome for a new operator.
// A NOT NULL column here would fail the insert on that entirely normal path.
//
// Disclaimer is deliberately not a column: it is a constant, identical on every
// card, and storing it would duplicate one sentence per row (docs/erd.md §6).
type Inspection struct {
	ID                string    `gorm:"column:id;primaryKey"`
	MachineID         string    `gorm:"column:machine_id"`
	BaselineID        string    `gorm:"column:baseline_id"`
	Status            string    `gorm:"column:status"`
	ZScore            *float64  `gorm:"column:z_score"`
	HealthScore       *float64  `gorm:"column:health_score"`
	DominantIndicator *string   `gorm:"column:dominant_indicator"`
	Reason            *string   `gorm:"column:reason"`
	AudioPath         *string   `gorm:"column:audio_path"`
	InspectedAt       time.Time `gorm:"column:inspected_at"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (Inspection) TableName() string { return "inspections" }
