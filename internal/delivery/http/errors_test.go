package http

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"audiax/internal/apperr"
	"audiax/internal/model"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The error handler is the only translation point between domain errors and
// HTTP, so a wrong mapping here leaks a 500 for every business failure.
func TestErrorHandlerMapsDomainErrorsToStatuses(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"not found", apperr.ErrNotFound, fiber.StatusNotFound, "resource not found"},
		{"conflict", apperr.ErrConflict, fiber.StatusConflict, "resource already exists"},
		{"unauthorized", apperr.ErrUnauthorized, fiber.StatusUnauthorized, "unauthorized"},
		{"forbidden", apperr.ErrForbidden, fiber.StatusForbidden, "forbidden"},
		{"wrapped sentinel", fmt.Errorf("find user: %w", apperr.ErrNotFound), fiber.StatusNotFound, "resource not found"},
		{"fiber error", fiber.NewError(fiber.StatusBadRequest, "malformed request body"), fiber.StatusBadRequest, "malformed request body"},
		{"unknown error", fmt.Errorf("connection reset by peer"), fiber.StatusInternalServerError, "internal server error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, body := do(t, tc.err)

			assert.Equal(t, tc.wantStatus, status)
			assert.Equal(t, tc.wantBody, body.Error)
			assert.Empty(t, body.Fields)
		})
	}
}

// An unknown error must never echo its text back: that is how connection
// strings and stack details end up in a client response.
func TestErrorHandlerHidesUnknownErrorDetail(t *testing.T) {
	_, body := do(t, fmt.Errorf("dial tcp 10.0.0.1:5432: password=hunter2"))

	assert.NotContains(t, body.Error, "hunter2")
	assert.NotContains(t, body.Error, "10.0.0.1")
}

func TestErrorHandlerReportsEachInvalidField(t *testing.T) {
	status, body := do(t, &apperr.ValidationError{Fields: map[string]string{
		"email":    "must be a valid email address",
		"password": "must be at least 8 characters",
	}})

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "validation failed", body.Error)
	assert.Equal(t, "must be a valid email address", body.Fields["email"])
	assert.Equal(t, "must be at least 8 characters", body.Fields["password"])
}

func do(t *testing.T, err error) (int, model.ErrorResponse) {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: NewErrorHandler(slog.New(slog.NewTextHandler(io.Discard, nil))),
	})
	app.Get("/boom", func(ctx *fiber.Ctx) error { return err })

	response, testErr := app.Test(httptest.NewRequest(fiber.MethodGet, "/boom", nil))
	require.NoError(t, testErr)
	defer response.Body.Close()

	raw, testErr := io.ReadAll(response.Body)
	require.NoError(t, testErr)

	var body model.ErrorResponse
	require.NoError(t, json.Unmarshal(raw, &body))

	return response.StatusCode, body
}
