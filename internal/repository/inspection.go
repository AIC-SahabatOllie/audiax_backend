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

// FindByIDForMachine scopes by machine_id in the same query, not as a
// separate ownership check afterwards: a client cannot use this to probe for
// the existence of another machine's inspection id.
func (r *InspectionRepository) FindByIDForMachine(db *gorm.DB, inspection *entity.Inspection, id, machineID string) error {
	return db.Where("id = ? and machine_id = ?", id, machineID).Take(inspection).Error
}
