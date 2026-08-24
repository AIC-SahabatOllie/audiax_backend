package model

// AdvisoryTurn is one exchange the client already has in its own local
// conversation history. The backend stores no conversation of its own
// (DESIGN.md §10): the client owns and resends this on every turn.
type AdvisoryTurn struct {
	Role    string `json:"role" validate:"required,oneof=user assistant"`
	Content string `json:"content" validate:"required"`
}

// AdvisoryContext is machine attributes the client sends because
// entity.Machine has no column for them yet (DESIGN.md §3.4, jebakan #3).
// Once migration 0005_add_machine_attributes lands this becomes optional.
type AdvisoryContext struct {
	DriveType   string `json:"drive_type" validate:"required,oneof=belt direct-coupled direct-drive"`
	Recency     string `json:"recency" validate:"required,oneof=<1bln 1-6bln >6bln tidak-tahu"`
	MachineAge  string `json:"machine_age" validate:"required,oneof=<1th 1-3th 3-5th >5th"`
	HoursPerDay string `json:"hours_per_day" validate:"required,oneof=<4 4-8 >8"`
	HasBackup   bool   `json:"has_backup"`
	LoadState   string `json:"load_state" validate:"required,oneof=kosong bermuatan"`
}

// AdvisoryMessageRequest is the body of one Teknisi Saku turn. UserID,
// MachineID and InspectionID carry no json tag: the controller fills them in
// from the session and the URL, never from client-supplied JSON, so a client
// can never claim ownership of an inspection it does not have (DESIGN.md
// §3.4, jebakan #1). The clinical facts (status, z-score, ...) are not part
// of this request at all -- the use case reads them from the inspection row.
type AdvisoryMessageRequest struct {
	UserID       string `validate:"required"`
	MachineID    string `validate:"required"`
	InspectionID string `validate:"required"`

	History     []AdvisoryTurn  `json:"history"`
	UserMessage string          `json:"user_message" validate:"required"`
	Context     AdvisoryContext `json:"context" validate:"required"`
}

// AdvisoryMessageResponse mirrors DESIGN.md §3.4. Source must always be shown
// by the client (a small badge): degradation to the no-LLM fallback has to be
// visible, never silent.
type AdvisoryMessageResponse struct {
	Reply           string `json:"reply"`
	NextStep        string `json:"next_step"`
	NeedsTechnician bool   `json:"needs_technician"`
	Escalated       bool   `json:"escalated"`
	Source          string `json:"source"`
	Disclaimer      string `json:"disclaimer"`
}
