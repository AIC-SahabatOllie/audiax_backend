package model

import "time"

// MachineResponse deliberately omits UserID: the caller already knows who they
// are, and echoing it back adds nothing a client can use.
type MachineResponse struct {
	ID          string    `json:"id"`
	Label       string    `json:"label"`
	Location    *string   `json:"location"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateMachineRequest struct {
	UserID      string  `json:"-" validate:"required"`
	Label       string  `json:"label" validate:"required,min=1,max=100"`
	Location    *string `json:"location,omitempty" validate:"omitempty,max=150"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000"`
}

// UpdateMachineRequest uses pointers so "field absent" is distinguishable from
// "field set to empty".
type UpdateMachineRequest struct {
	UserID      string  `json:"-" validate:"required"`
	MachineID   string  `json:"-" validate:"required"`
	Label       *string `json:"label,omitempty" validate:"omitempty,min=1,max=100"`
	Location    *string `json:"location,omitempty" validate:"omitempty,max=150"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000"`
}
