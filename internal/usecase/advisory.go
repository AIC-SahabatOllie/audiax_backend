package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"audiax/internal/advisory"
	"audiax/internal/apperr"
	"audiax/internal/constants"
	"audiax/internal/entity"
	"audiax/internal/model"

	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

type AdvisoryUseCase struct {
	db          *gorm.DB
	log         *slog.Logger
	validate    *validator.Validate
	machines    MachineRepository
	baselines   BaselineRepository
	inspections InspectionRepository
	llm         advisory.LLMProvider
	timeout     time.Duration
}

func NewAdvisoryUseCase(db *gorm.DB, log *slog.Logger, validate *validator.Validate,
	machines MachineRepository, baselines BaselineRepository, inspections InspectionRepository,
	llm advisory.LLMProvider, timeout time.Duration) *AdvisoryUseCase {

	return &AdvisoryUseCase{
		db: db, log: log, validate: validate,
		machines: machines, baselines: baselines, inspections: inspections,
		llm: llm, timeout: timeout,
	}
}

// Send answers one Teknisi Saku turn. The clinical facts (status, z-score,
// dominant indicator) are never taken from the request: they are read from
// the inspection row the client only references by id, with ownership
// checked through the machine it belongs to. A client that could supply its
// own status could claim NORMAL on a CRITICAL machine and the whole safety
// chain would collapse (DESIGN.md §3.4, jebakan #1).
func (u *AdvisoryUseCase) Send(ctx context.Context, request *model.AdvisoryMessageRequest) (*model.AdvisoryMessageResponse, error) {
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

	inspection := new(entity.Inspection)
	if err := u.inspections.FindByIDForMachine(db, inspection, request.InspectionID, machine.ID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find inspection by id: %w", err)
	}

	calibrationQuality := u.calibrationQualityFor(ctx, db, inspection)
	dominantIndicator := ""
	if inspection.DominantIndicator != nil {
		dominantIndicator = *inspection.DominantIndicator
	}

	// Danger keywords are checked deterministically, before the LLM is even
	// asked, and win regardless of the acoustic status (DESIGN.md §3.5,
	// jebakan #4). The LLM is not just outvoted here -- it is not called at
	// all, so there is no guard heuristic standing between an operator who
	// just reported smoke and the safety instruction.
	if containsDangerKeyword(request.UserMessage) {
		cell, err := advisory.Lookup(constants.StatusCritical, dominantIndicator,
			request.Context.DriveType, request.Context.Recency)
		if err != nil {
			return nil, fmt.Errorf("advisory: decision table lookup: %w", err)
		}
		reply := advisory.StaticReply(cell)
		reply.Escalated = true

		u.log.WarnContext(ctx, "advisory: danger keyword forced critical path",
			"inspection_id", inspection.ID, "acoustic_status", inspection.Status)

		return toResponse(reply, constants.AdvisorySourceFallbackStatic), nil
	}

	cell, err := advisory.Lookup(inspection.Status, dominantIndicator,
		request.Context.DriveType, request.Context.Recency)
	if err != nil {
		return nil, fmt.Errorf("advisory: decision table lookup: %w", err)
	}

	reply, source := u.reply(ctx, cell, inspection, calibrationQuality, request)

	u.log.InfoContext(ctx, "advisory message answered",
		"inspection_id", inspection.ID, "source", source, "needs_technician", reply.NeedsTechnician)

	return toResponse(reply, source), nil
}

// calibrationQualityFor joins calibration_quality from the baseline this
// specific inspection was scored against -- not necessarily the machine's
// currently active baseline, since it may have been recalibrated since
// (jebakan #2: inspections has no calibration_quality column of its own). A
// failed join omits the fact from the prompt entirely; it never renders
// "null" (PROMPT_CONTRACT.md rule 2).
func (u *AdvisoryUseCase) calibrationQualityFor(ctx context.Context, db *gorm.DB, inspection *entity.Inspection) string {
	baseline := new(entity.Baseline)
	if err := u.baselines.FindByID(db, baseline, inspection.BaselineID); err != nil {
		u.log.WarnContext(ctx, "advisory: baseline join failed, omitting calibration_quality from prompt",
			"inspection_id", inspection.ID, "baseline_id", inspection.BaselineID, "error", err)
		return ""
	}
	return baseline.CalibrationQuality
}

// reply tries the LLM and falls back to StaticReply on any failure: an
// unreachable provider, a timeout, or a guard rejection all degrade the same
// way (DESIGN.md decision 8). FAKTA in the rendered prompt always carries the
// true acoustic status, even though cell may reflect a forced override --
// the facts must never be edited, only the decision layered on top of them.
func (u *AdvisoryUseCase) reply(ctx context.Context, cell advisory.Cell, inspection *entity.Inspection,
	calibrationQuality string, request *model.AdvisoryMessageRequest) (advisory.Reply, string) {

	prompt := advisory.RenderPrompt(advisory.PromptInput{
		Status:             inspection.Status,
		Z:                  inspection.ZScore,
		ZWarningThreshold:  constants.AdvisoryZWarningThreshold,
		ZCriticalThreshold: constants.AdvisoryZCriticalThreshold,
		DominantIndicator:  derefOrEmpty(inspection.DominantIndicator),
		CalibrationQuality: calibrationQuality,
		DriveType:          request.Context.DriveType,
		Recency:            request.Context.Recency,
		MachineAge:         request.Context.MachineAge,
		HoursPerDay:        request.Context.HoursPerDay,
		HasBackup:          request.Context.HasBackup,
		LoadState:          request.Context.LoadState,
		Cell:               cell,
		History:            toAdvisoryTurns(request.History),
		UserMessage:        request.UserMessage,
	})

	callCtx, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()

	completion, err := u.llm.Complete(callCtx, prompt)
	if err != nil {
		u.log.WarnContext(ctx, "advisory: llm unavailable, falling back to static reply", "error", err)
		return advisory.StaticReply(cell), constants.AdvisorySourceFallbackStatic
	}

	reply, err := advisory.Guard(completion, cell, allowedNumbersFor(inspection))
	if err != nil {
		u.log.WarnContext(ctx, "advisory: guard rejected llm output, falling back to static reply", "error", err)
		return advisory.StaticReply(cell), constants.AdvisorySourceFallbackStatic
	}

	return reply, constants.AdvisorySourceLLM
}

// allowedNumbersFor is the guard's numeric whitelist contribution from the
// HealthCard: the z-score actually measured, plus the two static thresholds
// the prompt displays alongside it.
func allowedNumbersFor(inspection *entity.Inspection) []float64 {
	allowed := []float64{constants.AdvisoryZWarningThreshold, constants.AdvisoryZCriticalThreshold}
	if inspection.ZScore != nil {
		allowed = append(allowed, *inspection.ZScore)
	}
	return allowed
}

func toAdvisoryTurns(turns []model.AdvisoryTurn) []advisory.Turn {
	out := make([]advisory.Turn, len(turns))
	for i, t := range turns {
		out[i] = advisory.Turn{Role: t.Role, Content: t.Content}
	}
	return out
}

func toResponse(reply advisory.Reply, source string) *model.AdvisoryMessageResponse {
	return &model.AdvisoryMessageResponse{
		Reply:           reply.Answer,
		NextStep:        reply.NextStep,
		NeedsTechnician: reply.NeedsTechnician,
		Escalated:       reply.Escalated,
		Source:          source,
		Disclaimer:      constants.TriageDisclaimer,
	}
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// containsDangerKeyword matches constants.DangerKeywords case-insensitively
// against the operator's own message (DESIGN.md §3.5). This is deliberately
// separate from guard.go's diagnosis check: that validates what the LLM is
// about to say, this validates what the operator just reported, and the two
// run at different points in the flow.
func containsDangerKeyword(message string) bool {
	lower := strings.ToLower(message)
	for _, keyword := range constants.DangerKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}
