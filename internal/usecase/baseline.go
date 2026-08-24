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

type BaselineRepository interface {
	Create(db *gorm.DB, baseline *entity.Baseline) error
	FindActiveForMachine(db *gorm.DB, baseline *entity.Baseline, machineID string) error
	DeactivateAllForMachine(db *gorm.DB, machineID string) error
	ListForMachine(db *gorm.DB, baselines *[]entity.Baseline, machineID string) error
	UpdateAudioPath(db *gorm.DB, baselineID, path string) error
}

// AIService is the stateless inference service in ../audiax_model. Declared
// here, at the consumer, so this package never imports its implementation and
// stays testable with fakes.
type AIService interface {
	Calibrate(ctx context.Context, audio []byte, filename, machineLabel string) (*model.AIBaseline, error)
	Inspect(ctx context.Context, audio []byte, filename string, baselineJSON []byte) (*model.AIHealthCard, error)
}

type ObjectStore interface {
	Put(ctx context.Context, path string, content []byte, contentType string) error
}

type BaselineUseCase struct {
	db        *gorm.DB
	log       *slog.Logger
	validate  *validator.Validate
	machines  MachineRepository
	baselines BaselineRepository
	ai        AIService
	store     ObjectStore
}

func NewBaselineUseCase(db *gorm.DB, log *slog.Logger, validate *validator.Validate,
	machines MachineRepository, baselines BaselineRepository,
	ai AIService, store ObjectStore) *BaselineUseCase {

	return &BaselineUseCase{
		db: db, log: log, validate: validate,
		machines: machines, baselines: baselines, ai: ai, store: store,
	}
}

// Calibrate builds a new baseline for a machine from a healthy-condition
// recording and makes it the active one. The previous baseline is deactivated,
// never overwritten: a past z-score means nothing without the baseline that
// produced it (docs/erd.md §5.4).
func (u *BaselineUseCase) Calibrate(ctx context.Context, request *model.CalibrateMachineRequest) (*model.BaselineResponse, error) {
	if err := validateStruct(u.validate, request); err != nil {
		return nil, err
	}

	db := u.db.WithContext(ctx)

	machine, err := u.findOwnedMachineForBaseline(db, request.MachineID, request.UserID)
	if err != nil {
		return nil, err
	}

	// The AI service is called before the transaction opens. Calibration can
	// take a minute on a cold CPU, and holding a pooled connection open through
	// that would starve every other request.
	aiBaseline, err := u.ai.Calibrate(ctx, request.Audio, request.Filename, machine.Label)
	if err != nil {
		return nil, err
	}

	baseline, err := baselineEntity(machine.ID, aiBaseline)
	if err != nil {
		return nil, err
	}

	// Two statements, so a transaction. Deactivating first keeps the partial
	// unique index on (machine_id) where is_active satisfied at every point
	// inside it; the reverse order would violate it mid-transaction.
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := u.baselines.DeactivateAllForMachine(tx, machine.ID); err != nil {
			return fmt.Errorf("deactivate baselines: %w", err)
		}
		if err := u.baselines.Create(tx, baseline); err != nil {
			return fmt.Errorf("create baseline: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	u.storeCalibrationAudio(ctx, db, baseline, request.Audio)

	u.log.InfoContext(ctx, "machine calibrated",
		"machine_id", machine.ID,
		"baseline_id", baseline.ID,
		"calibration_quality", baseline.CalibrationQuality,
		"model_fingerprint", baseline.ModelFingerprint,
	)
	return converter.BaselineToResponse(baseline), nil
}

func (u *BaselineUseCase) List(ctx context.Context, userID, machineID string) ([]model.BaselineResponse, error) {
	db := u.db.WithContext(ctx)

	if _, err := u.findOwnedMachineForBaseline(db, machineID, userID); err != nil {
		return nil, err
	}

	var baselines []entity.Baseline
	if err := u.baselines.ListForMachine(db, &baselines, machineID); err != nil {
		return nil, fmt.Errorf("list baselines: %w", err)
	}
	return converter.BaselinesToResponses(baselines), nil
}

// findOwnedMachineForBaseline is the ownership gate for everything in this file.
// A machine belonging to someone else is reported as not found, so the API never
// confirms that an id the caller does not own exists.
func (u *BaselineUseCase) findOwnedMachineForBaseline(db *gorm.DB, machineID, userID string) (*entity.Machine, error) {
	machine := new(entity.Machine)
	if err := u.machines.FindByIDForUser(db, machine, machineID, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find machine by id: %w", err)
	}
	return machine, nil
}

// storeCalibrationAudio uploads the recording and records its path. A failure is
// logged, never returned: the audio is insurance for re-calibrating after a
// model upgrade, and losing it must not cost the operator a 120-second recording
// that has already been turned into a valid baseline.
//
// It goes through the repository rather than issuing an Update of its own. A use
// case that writes SQL has quietly become a repository, and the next person who
// needs that write will not find it where they look.
func (u *BaselineUseCase) storeCalibrationAudio(ctx context.Context, db *gorm.DB, baseline *entity.Baseline, audio []byte) {
	path := constants.CalibrationAudioPrefix + baseline.MachineID + "/" + baseline.ID + ".wav"

	if err := u.store.Put(ctx, path, audio, constants.AudioContentType); err != nil {
		u.log.ErrorContext(ctx, "calibration audio not stored",
			"baseline_id", baseline.ID, "error", err)
		return
	}

	if err := u.baselines.UpdateAudioPath(db, baseline.ID, path); err != nil {
		u.log.ErrorContext(ctx, "calibration audio path not recorded",
			"baseline_id", baseline.ID, "error", err)
		return
	}
	baseline.CalibrationAudioPath = &path
}

// baselineEntity converts the AI service's wire format into a row. The dtype and
// shape checks mirror MachineBaseline.from_json_dict: catching a malformed
// payload here yields an error that names the problem, where letting it reach
// the database yields a constraint violation that does not.
func baselineEntity(machineID string, ai *model.AIBaseline) (*entity.Baseline, error) {
	if ai.EmbeddingDtype != constants.EmbeddingDtypeFloat16 {
		return nil, fmt.Errorf("unsupported embedding dtype %q from ai service", ai.EmbeddingDtype)
	}
	if len(ai.EmbeddingShape) != 2 {
		return nil, fmt.Errorf("embedding_shape must be [N, D], got %v", ai.EmbeddingShape)
	}

	embeddings, err := base64.StdEncoding.DecodeString(ai.EmbeddingsB64)
	if err != nil {
		return nil, fmt.Errorf("decode embeddings: %w", err)
	}

	// Only embedding_shape[1] is kept. build_baseline always emits
	// embeddings.shape[0] == n_windows, so the first element is a duplicate of
	// a column we already store; the inspection path rebuilds the pair.
	dim := ai.EmbeddingShape[1]
	if expected := ai.NWindows * dim * 2; len(embeddings) != expected {
		return nil, fmt.Errorf("embeddings are %d bytes, expected %d for shape %v",
			len(embeddings), expected, ai.EmbeddingShape)
	}

	stats, err := json.Marshal(ai.BackendStats)
	if err != nil {
		return nil, fmt.Errorf("marshal backend_stats: %w", err)
	}
	notes, err := json.Marshal(orEmptyMap(ai.Notes))
	if err != nil {
		return nil, fmt.Errorf("marshal notes: %w", err)
	}

	calibratedAt, err := time.Parse(time.RFC3339, ai.CreatedAt)
	if err != nil {
		// The AI service's clock is not authoritative for our records. Fall back
		// rather than throw away a valid baseline over a timestamp format.
		calibratedAt = time.Now().UTC()
	}

	return &entity.Baseline{
		ID:                 uuid.NewString(),
		MachineID:          machineID,
		SchemaVersion:      ai.SchemaVersion,
		ModelFingerprint:   ai.ModelFingerprint,
		NWindows:           ai.NWindows,
		EmbeddingDim:       dim,
		EmbeddingDtype:     ai.EmbeddingDtype,
		Embeddings:         embeddings,
		BackendStats:       stats,
		Notes:              notes,
		CalibrationQuality: ai.CalibrationQuality,
		IsActive:           true,
		CalibratedAt:       calibratedAt,
	}, nil
}

// orEmptyMap keeps a null notes field from becoming a SQL NULL: the column is
// NOT NULL with a '{}' default, and an explicit null would violate it.
func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
