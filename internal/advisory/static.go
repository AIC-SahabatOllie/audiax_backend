package advisory

import (
	"fmt"
	"strings"
)

// Reply is the advisory package's own result type: the operator-facing
// message plus the two authoritative flags, regardless of whether it came
// from a guard-passed LLM completion (guard.go) or the no-LLM fallback
// (StaticReply). The caller decides the "source" label; this package does
// not know about HTTP or the LLM provider.
type Reply struct {
	Answer          string
	NextStep        string
	NeedsTechnician bool
	Escalated       bool
}

// StaticReply assembles a Reply directly from a decision table Cell, with no
// LLM involved. It is what the feature falls back to whenever the LLM is
// unavailable or its output fails a guard check (DESIGN.md decision 8:
// degrade honestly, never error). Every word in the result already passed
// human review as part of decision_table.json, so it needs no guard pass of
// its own.
//
// StaticReply never sets Escalated: it has no view of the operator's
// message, so only the deterministic danger-keyword check upstream is
// allowed to escalate.
func StaticReply(cell Cell) Reply {
	return Reply{
		Answer:          staticAnswer(cell),
		NextStep:        staticNextStep(cell),
		NeedsTechnician: cell.NeedsTechnician,
		Escalated:       false,
	}
}

func staticAnswer(cell Cell) string {
	var b strings.Builder

	if cell.SafetyGate != "" {
		b.WriteString(cell.SafetyGate)
	}

	for i, step := range cell.Checklist {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%d. %s", i+1, step)
	}

	if cell.EscalateIf != "" {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(cell.EscalateIf)
	}

	return b.String()
}

func staticNextStep(cell Cell) string {
	if len(cell.Checklist) > 0 {
		return cell.Checklist[0]
	}
	if cell.SafetyGate != "" {
		return cell.SafetyGate
	}
	return cell.EscalateIf
}
