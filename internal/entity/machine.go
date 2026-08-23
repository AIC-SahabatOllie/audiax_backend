package entity

import (
	"time"

	"gorm.io/gorm"
)

// Machine is a physical unit an operator monitors. Deletion is soft: inspections
// hold history that must survive removing a machine from the list.
type Machine struct {
	ID          string         `gorm:"column:id;primaryKey"`
	UserID      string         `gorm:"column:user_id"`
	Label       string         `gorm:"column:label"`
	Location    *string        `gorm:"column:location"`
	Description *string        `gorm:"column:description"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (Machine) TableName() string { return "machines" }
