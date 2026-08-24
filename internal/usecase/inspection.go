package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"audiax/internal/apperr"
	"audiax/internal/constants"
	"audiax/internal/entity"
	"audiax/internal/model"
	"audiax/internal/model/converter"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InspectionRepository interface {
	Create(db *gorm.DB, inspection *entity.Inspection) error
	ListForMachine(db *gorm.DB, inspections *[]entity.Inspection, machineID string, limit int) error
	// FindByIDForMachine scopes by machine_id, not just id: this is how
	// AdvisoryUseCase confirms an inspection actually belongs to the machine
	// the caller already proved they own, instead of trusting the client.
	FindByIDForMachine(db *gorm.DB, inspection *entity.Inspection, id, machineID string) error
}

type InspectionUseCase struct {
	db          *gorm.DB
	log         *slog.Logger
	validate    *validator.Validate
	machines    MachineRepository
	baselines   BaselineRepository
	inspections InspectionRepository
	ai          AIService
	store       ObjectStore
}

func NewInspectionUseCase(db *gorm.DB, log *slog.Logger, validate *validator.Validate,
	machines MachineRepository, baselines BaselineRepository, inspections InspectionRepository,
	ai AIService, store ObjectStore) *InspectionUseCase {

	return &InspectionUseCase{
		db: db, log: log, validate: validate,
		machines: machines, baselines: baselines, inspections: inspections,
		ai: ai, store: store,
	}
}

// Inspect scores one recording against the machine's active baseline. Synchronous
// by design: a 10-second clip does not justify a queue and a worker.
func (u *InspectionUseCase) Inspect(ctx context.Context, request *model.InspectMachineRequest) (*model.InspectionResponse, error) {
	if err := validateStruct(u.validate, request); err != nil {
		return nil, err
	}

	db := u.db.WithContext(ctx)

	machine := new(entity.Machine)
	if err := u.machines.FindByIDForUser(db, machine, request.MachineID, request.UserID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find machine by id: %w", err)
	}

	baseline := new(entity.Baseline)
	if err := u.baselines.FindActiveForMachine(db, baseline, machine.ID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Not ErrNotFound: the machine exists, it simply has no baseline yet.
			// The operator needs to be told what to do, not shown a 404.
			return nil, &apperr.RejectedError{
				Reason: "Mesin ini belum dikalibrasi. Rekam 2 menit kondisi sehat terlebih dahulu.",
			}
		}
		return nil, fmt.Errorf("find active baseline: %w", err)
	}

	baselineJSON, err := baselinePayload(machine.Label, baseline)
	if err != nil {
		return nil, err
	}

	card, err := u.ai.Inspect(ctx, request.Audio, request.Filename, baselineJSON)
	if err != nil {
		return nil, err
	}

	inspection := &entity.Inspection{
		ID:                uuid.NewString(),
		MachineID:         machine.ID,
		BaselineID:        baseline.ID,
		Status:            card.Status,
		ZScore:            card.ZScore,
		HealthScore:       card.HealthScore,
		DominantIndicator: card.DominantIndicator,
		Reason:            card.Reason,
		InspectedAt:       time.Now().UTC(),
	}

	// Only anomalous clips are kept, as evidence for whoever follows up. A
	// NORMAL clip is never re-inferred, so storing it buys nothing and costs
	// both money and privacy exposure (docs/prd.md FR10).
	if card.Status == constants.StatusWarning || card.Status == constants.StatusCritical {
		path := constants.InspectionAudioPrefix + machine.ID + "/" + inspection.ID + ".wav"
		if err := u.store.Put(ctx, path, request.Audio, constants.AudioContentType); err != nil {
			u.log.ErrorContext(ctx, "inspection audio not stored",
				"inspection_id", inspection.ID, "error", err)
		} else {
			inspection.AudioPath = &path
		}
	}

	// One statement, so no transaction (Rule 6).
	if err := u.inspections.Create(db, inspection); err != nil {
		return nil, fmt.Errorf("create inspection: %w", err)
	}

	u.log.InfoContext(ctx, "machine inspected",
		"machine_id", machine.ID,
		"inspection_id", inspection.ID,
		"status", inspection.Status,
	)

	// Echo the disclaimer the AI service returned; fall back to the constant if
	// it somehow came back empty, because a card without one must never ship.
	disclaimer := card.Disclaimer
	if disclaimer == "" {
		disclaimer = constants.TriageDisclaimer
	}
	return converter.InspectionToResponse(inspection, disclaimer), nil
}

func (u *InspectionUseCase) List(ctx context.Context, userID, machineID string) ([]model.InspectionResponse, error) {
	db := u.db.WithContext(ctx)

	machine := new(entity.Machine)
	if err := u.machines.FindByIDForUser(db, machine, machineID, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find machine by id: %w", err)
	}

	var inspections []entity.Inspection
	if err := u.inspections.ListForMachine(db, &inspections, machineID, constants.InspectionHistoryLimit); err != nil {
		return nil, fmt.Errorf("list inspections: %w", err)
	}
	return converter.InspectionsToResponses(inspections, constants.TriageDisclaimer), nil
}

// baselinePayload rebuilds the exact JSON MachineBaseline.from_json_dict expects.
// The machine label is read from the machine row rather than stored on the
// baseline: model_fingerprint does not cover the label, so renaming a machine
// can never invalidate its baseline.
func baselinePayload(machineLabel string, baseline *entity.Baseline) ([]byte, error) {
	var stats map[string]model.AIBackendStat
	if err := json.Unmarshal(baseline.BackendStats, &stats); err != nil {
		return nil, fmt.Errorf("unmarshal backend_stats: %w", err)
	}

	notes := map[string]any{}
	if len(baseline.Notes) > 0 {
		if err := json.Unmarshal(baseline.Notes, &notes); err != nil {
			return nil, fmt.Errorf("unmarshal notes: %w", err)
		}
	}

	payload := model.AIBaseline{
		SchemaVersion:      baseline.SchemaVersion,
		MachineLabel:       machineLabel,
		CreatedAt:          baseline.CalibratedAt.Format(time.RFC3339),
		ModelFingerprint:   baseline.ModelFingerprint,
		NWindows:           baseline.NWindows,
		EmbeddingShape:     []int{baseline.NWindows, baseline.EmbeddingDim},
		EmbeddingDtype:     baseline.EmbeddingDtype,
		EmbeddingsB64:      base64.StdEncoding.EncodeToString(baseline.Embeddings),
		BackendStats:       stats,
		CalibrationQuality: baseline.CalibrationQuality,
		Notes:              notes,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal baseline payload: %w", err)
	}
	return encoded, nil
}
