package advisory

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"audiax/internal/constants"
)

func float64Ptr(v float64) *float64 { return &v }

func fullPromptInput() PromptInput {
	cell, _ := Lookup(constants.StatusWarning, "crest_factor", "belt", ">6bln")
	return PromptInput{
		Status:             constants.StatusWarning,
		Z:                  float64Ptr(3.4),
		ZWarningThreshold:  3.0,
		ZCriticalThreshold: 6.0,
		DominantIndicator:  "crest_factor",
		CalibrationQuality: constants.CalibrationQualityGood,
		DriveType:          "belt",
		Recency:            ">6bln",
		MachineAge:         "3-5th",
		HoursPerDay:        ">8",
		HasBackup:          false,
		LoadState:          "bermuatan",
		Cell:               cell,
		History:            []Turn{{Role: "user", Content: "crest factor itu apa?"}, {Role: "assistant", Content: "Itu ukuran ketajaman puncak suara."}},
		UserMessage:        "sabuknya udah kenceng kok, tapi tadi ada bau gosong dikit",
	}
}

func TestRenderPromptIncludesFactsAndDecisionAndQuestion(t *testing.T) {
	out := RenderPrompt(fullPromptInput())

	assert.True(t, strings.Contains(out, "status: WARNING"))
	assert.True(t, strings.Contains(out, "z: 3.4 (ambang: warning 3.0, kritis 6.0)"))
	assert.True(t, strings.Contains(out, "indikator_dominan: crest_factor"))
	assert.True(t, strings.Contains(out, "urgensi: rencanakan_dalam_48_jam"))
	assert.True(t, strings.Contains(out, "PERTANYAAN: sabuknya udah kenceng kok, tapi tadi ada bau gosong dikit"))
}

func TestRenderPromptOmitsZLineWhenNil(t *testing.T) {
	in := fullPromptInput()
	in.Z = nil

	out := RenderPrompt(in)

	for _, line := range strings.Split(out, "\n") {
		assert.False(t, strings.HasPrefix(strings.TrimSpace(line), "z:"),
			"z: line must be removed entirely when z is nil, got line %q", line)
	}
}

func TestRenderPromptOmitsDominantIndicatorLineWhenEmpty(t *testing.T) {
	in := fullPromptInput()
	in.DominantIndicator = ""

	out := RenderPrompt(in)

	assert.False(t, strings.Contains(out, "indikator_dominan:"))
}

func TestRenderPromptOmitsCalibrationQualityLineWhenEmpty(t *testing.T) {
	in := fullPromptInput()
	in.CalibrationQuality = ""

	out := RenderPrompt(in)

	assert.False(t, strings.Contains(out, "kualitas_kalibrasi:"))
}

func TestRenderPromptFormatsHasBackupAsYaOrTidakNotBool(t *testing.T) {
	in := fullPromptInput()
	in.HasBackup = true
	out := RenderPrompt(in)
	assert.True(t, strings.Contains(out, "ada_cadangan: ya"))

	in.HasBackup = false
	out = RenderPrompt(in)
	assert.True(t, strings.Contains(out, "ada_cadangan: tidak"))
}

func TestRenderPromptNumbersChecklistWithFourSpaceIndent(t *testing.T) {
	in := fullPromptInput()
	require.NotEmpty(t, in.Cell.Checklist)

	out := RenderPrompt(in)

	assert.True(t, strings.Contains(out, "    1. "+in.Cell.Checklist[0]))
}

func TestRenderPromptOmitsHistoryBlockWhenEmpty(t *testing.T) {
	in := fullPromptInput()
	in.History = nil

	out := RenderPrompt(in)

	assert.False(t, strings.Contains(out, "RIWAYAT:"))
}

func TestRenderPromptFormatsHistoryWithOperatorAndAsistenPrefixes(t *testing.T) {
	out := RenderPrompt(fullPromptInput())

	assert.True(t, strings.Contains(out, "  operator: crest factor itu apa?"))
	assert.True(t, strings.Contains(out, "  asisten: Itu ukuran ketajaman puncak suara."))
}

func TestRenderPromptCapsHistoryAtMaxTurns(t *testing.T) {
	in := fullPromptInput()
	in.History = nil
	for i := 0; i < constants.AdvisoryMaxHistoryTurns+3; i++ {
		in.History = append(in.History, Turn{Role: "user", Content: "pesan lama"})
	}
	in.History = append(in.History, Turn{Role: "user", Content: "pesan terbaru"})

	out := RenderPrompt(in)

	assert.Equal(t, constants.AdvisoryMaxHistoryTurns, strings.Count(out, "operator: pesan lama")+strings.Count(out, "operator: pesan terbaru"))
	assert.True(t, strings.Contains(out, "pesan terbaru"), "the most recent turn must survive the cap, not the oldest")
}

func TestRenderPromptEndsWithExactlyOneNewlineAndNoTrailingSpaces(t *testing.T) {
	out := RenderPrompt(fullPromptInput())

	assert.True(t, strings.HasSuffix(out, "\n"))
	assert.False(t, strings.HasSuffix(out, "\n\n"))
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		assert.Equal(t, line, strings.TrimRight(line, " \t"), "line has trailing whitespace: %q", line)
	}
}

func TestTemplateHashIsA64CharHexDigest(t *testing.T) {
	assert.Len(t, TemplateHash, 64)
	assert.Regexp(t, "^[0-9a-f]{64}$", TemplateHash)
}
