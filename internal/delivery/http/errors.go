package http

import (
	"errors"
	"log/slog"

	"audiax/internal/apperr"
	"audiax/internal/constants"
	"audiax/internal/model"

	"github.com/gofiber/fiber/v2"
)

// NewErrorHandler is the single place where a domain error becomes an HTTP
// status. Use cases stay free of transport concerns because of it.
func NewErrorHandler(log *slog.Logger) fiber.ErrorHandler {
	return func(ctx *fiber.Ctx, err error) error {
		var validationErr *apperr.ValidationError
		if errors.As(err, &validationErr) {
			return ctx.Status(fiber.StatusBadRequest).JSON(model.ErrorResponse{
				Error:  "validation failed",
				Fields: validationErr.Fields,
			})
		}

		switch {
		case errors.Is(err, apperr.ErrNotFound):
			return respond(ctx, fiber.StatusNotFound, "resource not found")
		case errors.Is(err, apperr.ErrConflict):
			return respond(ctx, fiber.StatusConflict, "resource already exists")
		case errors.Is(err, apperr.ErrUnauthorized):
			return respond(ctx, fiber.StatusUnauthorized, "unauthorized")
		case errors.Is(err, apperr.ErrForbidden):
			return respond(ctx, fiber.StatusForbidden, "forbidden")
		}

		var fiberErr *fiber.Error
		if errors.As(err, &fiberErr) {
			return respond(ctx, fiberErr.Code, fiberErr.Message)
		}

		// Anything unrecognised is a bug: log it in full, tell the client nothing.
		log.ErrorContext(ctx.UserContext(), "unhandled error",
			"error", err,
			"method", ctx.Method(),
			"path", ctx.Path(),
			"request_id", ctx.Locals(constants.RequestIDLocalsKey),
		)
		return respond(ctx, fiber.StatusInternalServerError, "internal server error")
	}
}

func respond(ctx *fiber.Ctx, status int, message string) error {
	return ctx.Status(status).JSON(model.ErrorResponse{Error: message})
}
