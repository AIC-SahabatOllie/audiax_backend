package converter

import (
	"audiax/internal/entity"
	"audiax/internal/model"
)

func MachineToResponse(machine *entity.Machine) *model.MachineResponse {
	return &model.MachineResponse{
		ID:          machine.ID,
		Label:       machine.Label,
		Location:    machine.Location,
		Description: machine.Description,
		CreatedAt:   machine.CreatedAt,
		UpdatedAt:   machine.UpdatedAt,
	}
}

func MachinesToResponses(machines []entity.Machine) []model.MachineResponse {
	out := make([]model.MachineResponse, 0, len(machines))
	for i := range machines {
		out = append(out, *MachineToResponse(&machines[i]))
	}
	return out
}
