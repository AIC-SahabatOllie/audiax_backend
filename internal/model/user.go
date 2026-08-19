package model

import "time"

type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SessionResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type RegisterUserRequest struct {
	Email string `json:"email" validate:"required,email,max=254"`
	Name  string `json:"name" validate:"required,min=1,max=100"`
	// bcrypt silently ignores anything past constants.BcryptMaxPasswordBytes,
	// so reject it up front rather than accept a password whose tail is never
	// hashed. The literal below must stay in step with that constant; struct
	// tags cannot reference one.
	Password string `json:"password" validate:"required,min=8,max=72"`
}

type LoginUserRequest struct {
	Email    string `json:"email" validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,max=72"`
}

// UpdateUserRequest uses pointers so "field absent" is distinguishable from
// "field set to empty".
type UpdateUserRequest struct {
	UserID   string  `json:"-" validate:"required"`
	Name     *string `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	Password *string `json:"password,omitempty" validate:"omitempty,min=8,max=72"`
}
