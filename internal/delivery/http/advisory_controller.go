package http

import (
	"audiax/internal/delivery/http/middleware"
	"audiax/internal/model"
	"audiax/internal/usecase"

	"github.com/gofiber/fiber/v2"
)

type AdvisoryController struct {
	useCase *usecase.AdvisoryUseCase
}

func NewAdvisoryController(useCase *usecase.AdvisoryUseCase) *AdvisoryController {
	return &AdvisoryController{useCase: useCase}
}

func (c *AdvisoryController) Send(ctx *fiber.Ctx) error {
	request := new(model.AdvisoryMessageRequest)
	if err := ctx.BodyParser(request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "malformed request body")
	}
	request.UserID = middleware.GetAuth(ctx).UserID
	request.MachineID = ctx.Params("machineId")
	request.InspectionID = ctx.Params("inspectionId")

	response, err := c.useCase.Send(ctx.UserContext(), request)
	if err != nil {
		return err
	}
	// 200, not 201: a reply is computed, not stored -- Teknisi Saku keeps no
	// conversation of its own (DESIGN.md §10).
	return ctx.JSON(model.WebResponse[*model.AdvisoryMessageResponse]{Data: response})
}
