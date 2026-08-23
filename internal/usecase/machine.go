package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"audiax/internal/apperr"
	"audiax/internal/entity"
	"audiax/internal/model"
	"audiax/internal/model/converter"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Declared here, at the consumer, so this package never imports a concrete
// repository and can be unit tested with fakes.
type MachineRepository interface {
	Create(db *gorm.DB, machine *entity.Machine) error
	FindByIDForUser(db *gorm.DB, machine *entity.Machine, id, userID string) error
	ListForUser(db *gorm.DB, machines *[]entity.Machine, userID string) error
	Update(db *gorm.DB, machine *entity.Machine, fields map[string]any) error
	Delete(db *gorm.DB, machine *entity.Machine) error
}

type MachineUseCase struct {
	db       *gorm.DB
	log      *slog.Logger
	validate *validator.Validate
	machines MachineRepository
}

func NewMachineUseCase(db *gorm.DB, log *slog.Logger, validate *validator.Validate,
	machines MachineRepository) *MachineUseCase {

	return &MachineUseCase{db: db, log: log, validate: validate, machines: machines}
}

func (u *MachineUseCase) Create(ctx context.Context, request *model.CreateMachineRequest) (*model.MachineResponse, error) {
	// Normalise before validating: a label pasted with a trailing space is a
	// typo to clean up, not a request to reject.
	request.Label = strings.TrimSpace(request.Label)
	request.Location = trimOptional(request.Location)
	request.Description = trimOptional(request.Description)

	if err := validateStruct(u.validate, request); err != nil {
		return nil, err
	}

	machine := &entity.Machine{
		ID:          uuid.NewString(),
		UserID:      request.UserID,
		Label:       request.Label,
		Location:    request.Location,
		Description: request.Description,
	}

	// No pre-flight existence query: it cannot close the race between two
	// concurrent creates anyway. The partial unique index is the real guard,
	// so let the insert fail and translate it.
	if err := u.machines.Create(u.db.WithContext(ctx), machine); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperr.ErrConflict
		}
		return nil, fmt.Errorf("create machine: %w", err)
	}

	u.log.InfoContext(ctx, "machine created", "machine_id", machine.ID, "user_id", machine.UserID)
	return converter.MachineToResponse(machine), nil
}

func (u *MachineUseCase) List(ctx context.Context, userID string) ([]model.MachineResponse, error) {
	var machines []entity.Machine
	if err := u.machines.ListForUser(u.db.WithContext(ctx), &machines, userID); err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}
	return converter.MachinesToResponses(machines), nil
}

func (u *MachineUseCase) Get(ctx context.Context, userID, machineID string) (*model.MachineResponse, error) {
	machine, err := u.findOwnedMachine(u.db.WithContext(ctx), machineID, userID)
	if err != nil {
		return nil, err
	}
	return converter.MachineToResponse(machine), nil
}

func (u *MachineUseCase) Update(ctx context.Context, request *model.UpdateMachineRequest) (*model.MachineResponse, error) {
	if request.Label != nil {
		trimmed := strings.TrimSpace(*request.Label)
		request.Label = &trimmed
	}
	request.Location = trimOptional(request.Location)
	request.Description = trimOptional(request.Description)

	if err := validateStruct(u.validate, request); err != nil {
		return nil, err
	}

	db := u.db.WithContext(ctx)

	machine, err := u.findOwnedMachine(db, request.MachineID, request.UserID)
	if err != nil {
		return nil, err
	}

	fields := map[string]any{}
	if request.Label != nil {
		fields["label"] = *request.Label
	}
	if request.Location != nil {
		fields["location"] = *request.Location
	}
	if request.Description != nil {
		fields["description"] = *request.Description
	}
	if len(fields) == 0 {
		return converter.MachineToResponse(machine), nil
	}

	// One statement, so no transaction (project-structure.md Rule 6).
	if err := u.machines.Update(db, machine, fields); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperr.ErrConflict
		}
		return nil, fmt.Errorf("update machine: %w", err)
	}

	if request.Label != nil {
		machine.Label = *request.Label
	}
	if request.Location != nil {
		machine.Location = request.Location
	}
	if request.Description != nil {
		machine.Description = request.Description
	}
	return converter.MachineToResponse(machine), nil
}

func (u *MachineUseCase) Delete(ctx context.Context, userID, machineID string) error {
	db := u.db.WithContext(ctx)

	machine, err := u.findOwnedMachine(db, machineID, userID)
	if err != nil {
		return err
	}

	// Soft delete: gorm.DeletedAt turns this into an UPDATE, so the inspection
	// history for this machine survives (docs/erd.md §5.3).
	if err := u.machines.Delete(db, machine); err != nil {
		return fmt.Errorf("delete machine: %w", err)
	}

	u.log.InfoContext(ctx, "machine deleted", "machine_id", machineID, "user_id", userID)
	return nil
}

// findOwnedMachine is the single ownership gate. Every machine-scoped operation
// goes through it, and a machine owned by someone else is reported as not found
// so the API never confirms an id the caller does not own.
func (u *MachineUseCase) findOwnedMachine(db *gorm.DB, machineID, userID string) (*entity.Machine, error) {
	machine := new(entity.Machine)
	if err := u.machines.FindByIDForUser(db, machine, machineID, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find machine by id: %w", err)
	}
	return machine, nil
}

func trimOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}
