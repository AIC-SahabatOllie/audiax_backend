package repository

import (
	"audiax/internal/entity"

	"gorm.io/gorm"
)

type InspectionRepository struct {
	Repository[entity.Inspection]
}

func NewInspectionRepository() *InspectionRepository { return &InspectionRepository{} }

// ListForMachine returns newest first, which is the order the trend chart and
// the history screen both want. The index on (machine_id, inspected_at desc)
// serves this exactly.
func (r *InspectionRepository) ListForMachine(db *gorm.DB, inspections *[]entity.Inspection, machineID string, limit int) error {
	return db.Where("machine_id = ?", machineID).
		Order("inspected_at desc").
		Limit(limit).
		Find(inspections).Error
}
