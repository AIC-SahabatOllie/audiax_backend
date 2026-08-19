package http

import (
	"audiax/internal/delivery/http/middleware"
	"audiax/internal/model"
	"audiax/internal/usecase"

	"github.com/gofiber/fiber/v2"
)

type UserController struct {
	useCase *usecase.UserUseCase
}

func NewUserController(useCase *usecase.UserUseCase) *UserController {
	return &UserController{useCase: useCase}
}

func (c *UserController) Register(ctx *fiber.Ctx) error {
	request := new(model.RegisterUserRequest)
	if err := ctx.BodyParser(request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "malformed request body")
	}

	response, err := c.useCase.Register(ctx.UserContext(), request)
	if err != nil {
		return err
	}
	return ctx.Status(fiber.StatusCreated).JSON(model.WebResponse[*model.UserResponse]{Data: response})
}

func (c *UserController) Login(ctx *fiber.Ctx) error {
	request := new(model.LoginUserRequest)
	if err := ctx.BodyParser(request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "malformed request body")
	}

	response, err := c.useCase.Login(ctx.UserContext(), request)
	if err != nil {
		return err
	}
	return ctx.JSON(model.WebResponse[*model.SessionResponse]{Data: response})
}

func (c *UserController) Current(ctx *fiber.Ctx) error {
	response, err := c.useCase.Current(ctx.UserContext(), middleware.GetAuth(ctx).UserID)
	if err != nil {
		return err
	}
	return ctx.JSON(model.WebResponse[*model.UserResponse]{Data: response})
}

func (c *UserController) Update(ctx *fiber.Ctx) error {
	request := new(model.UpdateUserRequest)
	if err := ctx.BodyParser(request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "malformed request body")
	}
	request.UserID = middleware.GetAuth(ctx).UserID

	response, err := c.useCase.Update(ctx.UserContext(), request)
	if err != nil {
		return err
	}
	return ctx.JSON(model.WebResponse[*model.UserResponse]{Data: response})
}

func (c *UserController) Logout(ctx *fiber.Ctx) error {
	if err := c.useCase.Logout(ctx.UserContext(), middleware.GetAuth(ctx).Token); err != nil {
		return err
	}
	return ctx.SendStatus(fiber.StatusNoContent)
}
