package usecase

import (
	"errors"
	"fmt"

	"audiax/internal/apperr"

	"github.com/go-playground/validator/v10"
)

// validateStruct turns validator's error list into an apperr.ValidationError so
// the client is told which field failed and why.
//
// Package-level rather than a method: every use case in this package needs it,
// and each one carrying its own copy would drift the moment a message changed.
func validateStruct(validate *validator.Validate, request any) error {
	err := validate.Struct(request)
	if err == nil {
		return nil
	}

	var invalid *validator.InvalidValidationError
	if errors.As(err, &invalid) {
		return fmt.Errorf("validate: %w", err)
	}

	var fieldErrors validator.ValidationErrors
	if !errors.As(err, &fieldErrors) {
		return fmt.Errorf("validate: %w", err)
	}

	fields := make(map[string]string, len(fieldErrors))
	for _, fe := range fieldErrors {
		fields[fe.Field()] = describe(fe)
	}
	return &apperr.ValidationError{Fields: fields}
}

func describe(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return "must be at least " + fe.Param() + " characters"
	case "max":
		return "must be at most " + fe.Param() + " characters"
	default:
		return "failed the " + fe.Tag() + " rule"
	}
}
