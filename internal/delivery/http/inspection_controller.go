package http

import (
	"audiax/internal/delivery/http/middleware"
	"audiax/internal/model"
	"audiax/internal/usecase"

	"github.com/gofiber/fiber/v2"
)

type InspectionController struct {
	useCase *usecase.InspectionUseCase
}

func NewInspectionController(useCase *usecase.InspectionUseCase) *InspectionController {
	return &InspectionController{useCase: useCase}
}

func (c *InspectionController) Inspect(ctx *fiber.Ctx) error {
	audio, filename, err := readAudioUpload(ctx)
	if err != nil {
		return err
	}

	response, err := c.useCase.Inspect(ctx.UserContext(), &model.InspectMachineRequest{
		UserID:    middleware.GetAuth(ctx).UserID,
		MachineID: ctx.Params("machineId"),
		Filename:  filename,
		Audio:     audio,
	})
	if err != nil {
		return err
	}
	// 201: an inspection is a record that now exists, whatever its verdict.
	// KALIBRASI_KURANG arrives here too -- it is an outcome, not an error.
	return ctx.Status(fiber.StatusCreated).JSON(model.WebResponse[*model.InspectionResponse]{Data: response})
}

func (c *InspectionController) List(ctx *fiber.Ctx) error {
	response, err := c.useCase.List(ctx.UserContext(),
		middleware.GetAuth(ctx).UserID, ctx.Params("machineId"))
	if err != nil {
		return err
	}
	return ctx.JSON(model.WebResponse[[]model.InspectionResponse]{Data: response})
}
