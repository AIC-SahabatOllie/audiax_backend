package usecase

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"audiax/internal/apperr"
	"audiax/internal/config"
	"audiax/internal/entity"
	"audiax/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
)

func newTestMachineUseCase(t *testing.T) (*MachineUseCase, *fakeMachineRepo) {
	t.Helper()

	// DummyDialector yields a *gorm.DB that supports WithContext without a server.
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	require.NoError(t, err)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	machines := newFakeMachineRepo()

	return NewMachineUseCase(db, log, config.NewValidator(), machines), machines
}

func TestCreateMachineTrimsLabelBeforeValidating(t *testing.T) {
	uc, machines := newTestMachineUseCase(t)

	response, err := uc.Create(context.Background(), &model.CreateMachineRequest{
		UserID: "user-1",
		Label:  "  Blower Oven 1  ",
	})

	require.NoError(t, err)
	assert.Equal(t, "Blower Oven 1", response.Label)
	assert.Equal(t, "Blower Oven 1", machines.byID[response.ID].Label)
}

func TestCreateMachineRejectsEmptyLabel(t *testing.T) {
	uc, _ := newTestMachineUseCase(t)

	_, err := uc.Create(context.Background(), &model.CreateMachineRequest{
		UserID: "user-1",
		Label:  "   ",
	})

	var validationErr *apperr.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, "is required", validationErr.Fields["label"])
}

func TestCreateMachineDuplicateLabelIsConflict(t *testing.T) {
	uc, _ := newTestMachineUseCase(t)
	ctx := context.Background()

	_, err := uc.Create(ctx, &model.CreateMachineRequest{UserID: "user-1", Label: "Blower Oven 1"})
	require.NoError(t, err)

	// The index is on lower(label), so case alone must not create a second one.
	_, err = uc.Create(ctx, &model.CreateMachineRequest{UserID: "user-1", Label: "blower oven 1"})
	assert.ErrorIs(t, err, apperr.ErrConflict)
}

func TestCreateMachineAllowsSameLabelForDifferentUsers(t *testing.T) {
	uc, _ := newTestMachineUseCase(t)
	ctx := context.Background()

	_, err := uc.Create(ctx, &model.CreateMachineRequest{UserID: "user-1", Label: "Blower Oven 1"})
	require.NoError(t, err)

	_, err = uc.Create(ctx, &model.CreateMachineRequest{UserID: "user-2", Label: "Blower Oven 1"})
	assert.NoError(t, err)
}

func TestGetMachineOwnedBySomeoneElseIsNotFound(t *testing.T) {
	uc, machines := newTestMachineUseCase(t)
	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-2", Label: "Theirs"}

	_, err := uc.Get(context.Background(), "user-1", "machine-1")

	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestListMachinesReturnsOnlyOwnMachines(t *testing.T) {
	uc, machines := newTestMachineUseCase(t)
	machines.byID["mine"] = &entity.Machine{ID: "mine", UserID: "user-1", Label: "Mine"}
	machines.byID["theirs"] = &entity.Machine{ID: "theirs", UserID: "user-2", Label: "Theirs"}

	response, err := uc.List(context.Background(), "user-1")

	require.NoError(t, err)
	require.Len(t, response, 1)
	assert.Equal(t, "mine", response[0].ID)
}

func TestUpdateMachineWritesOnlyProvidedFields(t *testing.T) {
	uc, machines := newTestMachineUseCase(t)
	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-1", Label: "Old"}

	newLabel := "  New Label  "
	response, err := uc.Update(context.Background(), &model.UpdateMachineRequest{
		UserID:    "user-1",
		MachineID: "machine-1",
		Label:     &newLabel,
	})

	require.NoError(t, err)
	// Only the named column is written: Save would overwrite every other one.
	assert.Equal(t, map[string]any{"label": "New Label"}, machines.updated)
	assert.Equal(t, "New Label", response.Label)
}

func TestUpdateMachineWithNoFieldsIsANoOp(t *testing.T) {
	uc, machines := newTestMachineUseCase(t)
	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-1", Label: "Old"}

	response, err := uc.Update(context.Background(), &model.UpdateMachineRequest{
		UserID:    "user-1",
		MachineID: "machine-1",
	})

	require.NoError(t, err)
	assert.Nil(t, machines.updated)
	assert.Equal(t, "Old", response.Label)
}

func TestDeleteMachineRemovesOnlyOwnedMachine(t *testing.T) {
	uc, machines := newTestMachineUseCase(t)
	machines.byID["machine-1"] = &entity.Machine{ID: "machine-1", UserID: "user-2", Label: "Theirs"}

	err := uc.Delete(context.Background(), "user-1", "machine-1")

	assert.ErrorIs(t, err, apperr.ErrNotFound)
	assert.Empty(t, machines.deleted)
}
