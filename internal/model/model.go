package model

// WebResponse is the envelope for every successful response.
type WebResponse[T any] struct {
	Data T `json:"data"`
}

// ErrorResponse is the envelope for every failed response. Fields is populated
// only for validation failures.
type ErrorResponse struct {
	Error  string            `json:"error"`
	Fields map[string]string `json:"fields,omitempty"`
}

// Auth is the authenticated caller, resolved by the auth middleware.
type Auth struct {
	UserID string
	Token  string
}
