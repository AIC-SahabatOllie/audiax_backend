package usecase

import (
	"strings"

	"audiax/internal/entity"

	"gorm.io/gorm"
)

// fakeMachineRepo keeps machines in a map, so these tests need no database.
// Shared by machine_test.go and, later, the baseline and inspection tests.
type fakeMachineRepo struct {
	byID    map[string]*entity.Machine
	updated map[string]any
	deleted []string
}

func newFakeMachineRepo() *fakeMachineRepo {
	return &fakeMachineRepo{byID: map[string]*entity.Machine{}}
}

// Create mirrors the partial unique index on (user_id, lower(label)): the
// database is what actually enforces this, and the fake has to agree or the
// conflict path would never be exercised.
func (f *fakeMachineRepo) Create(_ *gorm.DB, machine *entity.Machine) error {
	for _, existing := range f.byID {
		if existing.UserID == machine.UserID &&
			strings.EqualFold(existing.Label, machine.Label) {
			return gorm.ErrDuplicatedKey
		}
	}
	copied := *machine
	f.byID[machine.ID] = &copied
	return nil
}

func (f *fakeMachineRepo) FindByIDForUser(_ *gorm.DB, machine *entity.Machine, id, userID string) error {
	found, ok := f.byID[id]
	if !ok || found.UserID != userID {
		return gorm.ErrRecordNotFound
	}
	*machine = *found
	return nil
}

func (f *fakeMachineRepo) ListForUser(_ *gorm.DB, machines *[]entity.Machine, userID string) error {
	for _, m := range f.byID {
		if m.UserID == userID {
			*machines = append(*machines, *m)
		}
	}
	return nil
}

func (f *fakeMachineRepo) Update(_ *gorm.DB, _ *entity.Machine, fields map[string]any) error {
	f.updated = fields
	return nil
}

func (f *fakeMachineRepo) Delete(_ *gorm.DB, machine *entity.Machine) error {
	f.deleted = append(f.deleted, machine.ID)
	delete(f.byID, machine.ID)
	return nil
}
