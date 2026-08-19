package config

import (
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

func NewValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())

	// Report failures using the JSON field name so API clients see the name
	// they actually sent, not the Go struct field.
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "-" || name == "" {
			return field.Name
		}
		return name
	})
	return v
}
