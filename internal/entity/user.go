package entity

import "time"

// User is the database representation of a user. It never leaves the
// repository/use case boundary as-is: PasswordHash must not reach a client,
// so responses are built by the converter package instead.
type User struct {
	ID           string    `gorm:"column:id;primaryKey"`
	Email        string    `gorm:"column:email;uniqueIndex"`
	Name         string    `gorm:"column:name"`
	PasswordHash string    `gorm:"column:password_hash"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (User) TableName() string { return "users" }
