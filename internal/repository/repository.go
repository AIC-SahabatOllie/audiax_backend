package repository

import "gorm.io/gorm"

// Repository holds the operations that are identical for every entity.
// The *gorm.DB is passed in rather than stored so a use case can hand down
// either the plain connection or an open transaction.
type Repository[T any] struct{}

func (r *Repository[T]) Create(db *gorm.DB, entity *T) error {
	return db.Create(entity).Error
}

func (r *Repository[T]) FindByID(db *gorm.DB, entity *T, id any) error {
	return db.Where("id = ?", id).Take(entity).Error
}

// Update writes only the named columns. Save() would write every column,
// which turns a partial update into an accidental overwrite of the rest.
func (r *Repository[T]) Update(db *gorm.DB, entity *T, fields map[string]any) error {
	return db.Model(entity).Updates(fields).Error
}

func (r *Repository[T]) Delete(db *gorm.DB, entity *T) error {
	return db.Delete(entity).Error
}
