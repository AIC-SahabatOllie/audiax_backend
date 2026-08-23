package repository

import (
	"audiax/internal/entity"

	"gorm.io/gorm"
)

type MachineRepository struct {
	Repository[entity.Machine]
}

func NewMachineRepository() *MachineRepository { return &MachineRepository{} }

// FindByIDForUser scopes the lookup to the owner. A machine belonging to someone
// else comes back as ErrRecordNotFound rather than a permission error, so the
// API never confirms that an id the caller does not own exists.
func (r *MachineRepository) FindByIDForUser(db *gorm.DB, machine *entity.Machine, id, userID string) error {
	return db.Where("id = ? and user_id = ?", id, userID).Take(machine).Error
}

func (r *MachineRepository) ListForUser(db *gorm.DB, machines *[]entity.Machine, userID string) error {
	return db.Where("user_id = ?", userID).Order("created_at desc").Find(machines).Error
}
