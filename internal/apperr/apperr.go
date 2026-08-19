// Package apperr defines the errors the business layer is allowed to speak.
//
// The use case layer must never return a transport error (fiber.ErrBadRequest
// and friends). Returning these sentinels instead keeps use cases usable from
// HTTP, gRPC, a CLI or a worker, and leaves status-code mapping to delivery.
package apperr

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound     = errors.New("resource not found")
	ErrConflict     = errors.New("resource already exists")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

// ValidationError carries per-field failures so a client learns which field
// was rejected instead of receiving a bare 400.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for field, msg := range e.Fields {
		parts = append(parts, fmt.Sprintf("%s: %s", field, msg))
	}
	return "validation failed (" + strings.Join(parts, ", ") + ")"
}
