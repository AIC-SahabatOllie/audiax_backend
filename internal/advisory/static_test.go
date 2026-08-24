package advisory

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"audiax/internal/constants"
)

func TestStaticReplyForNormalNeedsNoTechnicianAndIsNotEscalated(t *testing.T) {
	cell, err := Lookup(constants.StatusNormal, "kurtosis", "belt", "<1bln")
	require.NoError(t, err)

	reply := StaticReply(cell)

	assert.False(t, reply.NeedsTechnician)
	assert.False(t, reply.Escalated)
	assert.NotEmpty(t, reply.Answer)
	assert.NotEmpty(t, reply.NextStep)
}

func TestStaticReplyNextStepIsFirstChecklistItem(t *testing.T) {
	cell, err := Lookup(constants.StatusWarning, "crest_factor", "belt", "1-6bln")
	require.NoError(t, err)
	require.NotEmpty(t, cell.Checklist)

	reply := StaticReply(cell)

	assert.Equal(t, cell.Checklist[0], reply.NextStep)
}

func TestStaticReplyIncludesSafetyGateWhenPresent(t *testing.T) {
	cell, err := Lookup(constants.StatusCritical, "kurtosis", "belt", "<1bln")
	require.NoError(t, err)
	require.NotEmpty(t, cell.SafetyGate)

	reply := StaticReply(cell)

	assert.True(t, strings.Contains(reply.Answer, cell.SafetyGate))
	assert.True(t, reply.NeedsTechnician)
}

func TestStaticReplyNeverEscalatesOnItsOwn(t *testing.T) {
	// StaticReply has no view of the operator's message, so it must never set
	// Escalated itself -- only the deterministic danger-keyword check
	// (upstream, in the usecase) is allowed to do that.
	cell, err := Lookup(constants.StatusCritical, "kurtosis", "belt", "<1bln")
	require.NoError(t, err)

	reply := StaticReply(cell)

	assert.False(t, reply.Escalated)
}
