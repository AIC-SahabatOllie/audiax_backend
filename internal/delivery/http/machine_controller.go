package http

import (
	"audiax/internal/delivery/http/middleware"
	"audiax/internal/model"
	"audiax/internal/usecase"

	"github.com/gofiber/fiber/v2"
)

type MachineController struct {
	useCase *usecase.MachineUseCase
}

func NewMachineController(useCase *usecase.MachineUseCase) *MachineController {
	return &MachineController{useCase: useCase}
}

func (c *MachineController) Create(ctx *fiber.Ctx) error {
	request := new(model.CreateMachineRequest)
	if err := ctx.BodyParser(request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "malformed request body")
	}
	request.UserID = middleware.GetAuth(ctx).UserID

	response, err := c.useCase.Create(ctx.UserContext(), request)
	if err != nil {
		return err
	}
	return ctx.Status(fiber.StatusCreated).JSON(model.WebResponse[*model.MachineResponse]{Data: response})
}

func (c *MachineController) List(ctx *fiber.Ctx) error {
	response, err := c.useCase.List(ctx.UserContext(), middleware.GetAuth(ctx).UserID)
	if err != nil {
		return err
	}
	return ctx.JSON(model.WebResponse[[]model.MachineResponse]{Data: response})
}

func (c *MachineController) Get(ctx *fiber.Ctx) error {
	response, err := c.useCase.Get(ctx.UserContext(), middleware.GetAuth(ctx).UserID, ctx.Params("machineId"))
	if err != nil {
		return err
	}
	return ctx.JSON(model.WebResponse[*model.MachineResponse]{Data: response})
}

func (c *MachineController) Update(ctx *fiber.Ctx) error {
	request := new(model.UpdateMachineRequest)
	if err := ctx.BodyParser(request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "malformed request body")
	}
	request.UserID = middleware.GetAuth(ctx).UserID
	request.MachineID = ctx.Params("machineId")

	response, err := c.useCase.Update(ctx.UserContext(), request)
	if err != nil {
		return err
	}
	return ctx.JSON(model.WebResponse[*model.MachineResponse]{Data: response})
}

func (c *MachineController) Delete(ctx *fiber.Ctx) error {
	if err := c.useCase.Delete(ctx.UserContext(), middleware.GetAuth(ctx).UserID, ctx.Params("machineId")); err != nil {
		return err
	}
	return ctx.SendStatus(fiber.StatusNoContent)
}
