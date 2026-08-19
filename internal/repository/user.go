package repository

import (
	"audiax/internal/entity"

	"gorm.io/gorm"
)

type UserRepository struct {
	Repository[entity.User]
}

func NewUserRepository() *UserRepository { return &UserRepository{} }

func (r *UserRepository) FindByEmail(db *gorm.DB, user *entity.User, email string) error {
	return db.Where("email = ?", email).Take(user).Error
}
