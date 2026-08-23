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
	// ErrUnavailable means a dependency this request needed did not answer.
	// The caller may retry; nothing is wrong with the request itself.
	ErrUnavailable = errors.New("upstream service unavailable")
)

// RejectedError carries an actionable reason from an upstream service straight
// through to the operator. The AI quality gate produces messages like "rekam
// ulang lebih dekat ke mesin"; losing that text and returning a bare status
// would leave the operator with no idea what to do differently.
type RejectedError struct {
	Reason string
}

func (e *RejectedError) Error() string { return e.Reason }

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
