package advisory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"audiax/internal/constants"
)

// testdata/golden_prompt_no_history.txt holds a prompt rendered by the PYTHON
// side -- the same gen_corpus.py that produced every training example the model
// ever saw. Regenerate it from experiments/advisory/ in the audiax_model repo;
// never edit it by hand.
//
// Why it exists at all:
//
// prompt_template.txt is shared, but the two renderers are separate
// implementations in different languages. Nothing about that arrangement makes
// them agree -- they agree only as long as somebody checks. A drift here does
// not crash and does not fail any other test; the model simply starts seeing a
// format it was not trained on and answers a little worse, and the cause is
// almost impossible to find from the symptom.
//
// So the assertion below is deliberately strings.Equal on the WHOLE output, not
// Contains on pieces of it. Every other test in prompt_test.go checks a rule in
// isolation and would happily pass while an extra blank line, a lost indent, or
// an un-humanised enum quietly changed what production sends.

// readGolden loads a golden file, tolerating a CRLF checkout.
func readGolden(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err, "golden file missing -- regenerate it from the audiax_model repo, do not hand-write it")
	// Git on Windows may check the file out with CRLF; the renderer always
	// emits LF, and the difference is not what this test is about.
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

// goldenPromptInput is the exact scenario the golden file was rendered from.
// Fields carry RAW enum codes on purpose: that is what a caller reading from
// the database actually has, and humanising them is RenderPrompt's job.
func goldenPromptInput(t *testing.T) PromptInput {
	t.Helper()
	cell, err := Lookup(constants.StatusWarning, "crest_factor", "belt", ">6bln")
	require.NoError(t, err, "decision table lost the cell the golden file was built from")

	return PromptInput{
		Status:             constants.StatusWarning,
		Z:                  float64Ptr(3.4),
		ZWarningThreshold:  constants.AdvisoryZWarningThreshold,
		ZCriticalThreshold: constants.AdvisoryZCriticalThreshold,
		DominantIndicator:  "crest_factor",
		CalibrationQuality: constants.CalibrationQualityGood,
		DriveType:          "belt",
		Recency:            ">6bln",
		MachineAge:         "3-5th",
		HoursPerDay:        ">8",
		HasBackup:          false,
		LoadState:          "bermuatan",
		Cell:               cell,
		History:            nil,
		UserMessage:        "sabuknya udah kenceng kok, tapi tadi ada bau gosong dikit",
	}
}

// TestRenderPromptMatchesPythonGoldenByteForByte is the anti-skew gate.
// If it fails, do NOT edit the golden file to match Go. Work out which side
// changed, fix that side, and only then regenerate the golden.
func TestRenderPromptMatchesPythonGoldenByteForByte(t *testing.T) {
	want := readGolden(t, "golden_prompt_no_history.txt")
	got := RenderPrompt(goldenPromptInput(t))

	if got == want {
		return
	}

	// A plain Equal on a 900-byte string prints an unreadable blob, and the
	// difference is usually a single line. Report the first divergence.
	wantLines, gotLines := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		w, g := "<no line>", "<no line>"
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			t.Fatalf("prompt diverges from the training corpus at line %d\n"+
				"  python (what the model was trained on): %q\n"+
				"  go     (what production would send):    %q\n"+
				"total lines: python=%d go=%d\n"+
				"Fix the side that is wrong -- do not edit the golden file to match Go.",
				i+1, w, g, len(wantLines), len(gotLines))
		}
	}
	t.Fatalf("outputs differ but no line diverges -- trailing newline mismatch?\npython=%q\ngo=%q",
		want[max(0, len(want)-20):], got[max(0, len(got)-20):])
}

// TestRenderPromptHumanisesEnumCodes pins the substitutions individually, so a
// failure says which map is wrong instead of only that the golden moved.
func TestRenderPromptHumanisesEnumCodes(t *testing.T) {
	in := goldenPromptInput(t)
	out := RenderPrompt(in)

	assert.Contains(t, out, "penggerak: sabuk-puli", "drive_type must reach the model humanised, not as %q", in.DriveType)
	assert.Contains(t, out, "terakhir_dirawat: >6 bulan")
	assert.Contains(t, out, "umur_mesin: 3-5 tahun")

	assert.NotContains(t, out, "penggerak: belt")
	assert.NotContains(t, out, "terakhir_dirawat: >6bln")
	assert.NotContains(t, out, "umur_mesin: 3-5th")
}

// TestUrgencyIsNotHumanised guards the asymmetry. The corpus carries urgency
// with underscores straight from the decision table, so "helpfully" prettifying
// it here would be a regression, not a polish.
func TestUrgencyIsNotHumanised(t *testing.T) {
	out := RenderPrompt(goldenPromptInput(t))

	assert.Contains(t, out, "urgensi: rencanakan_dalam_48_jam")
	assert.NotContains(t, out, "urgensi: rencanakan dalam 48 jam")
}

// TestEveryDriveTypeAndRecencyInDecisionTableHasALabel walks decision_table.json
// and asserts every code it can produce is covered by a map. Without this, a new
// cell added to the table silently falls through humanise() unchanged and
// reintroduces the exact skew this file exists to prevent.
func TestEveryDriveTypeAndRecencyInDecisionTableHasALabel(t *testing.T) {
	raw, err := os.ReadFile("decision_table.json")
	require.NoError(t, err)

	var doc struct {
		Cells []struct {
			Key string `json:"key"`
		} `json:"cells"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.NotEmpty(t, doc.Cells, "decision table parsed to zero cells -- schema changed?")

	for _, cell := range doc.Cells {
		parts := strings.Split(cell.Key, "|")
		require.Len(t, parts, 4, "malformed key %q", cell.Key)

		if drive := parts[2]; drive != "*" {
			_, ok := driveLabel[drive]
			assert.True(t, ok, "decision table key %q uses drive_type %q with no entry in driveLabel; add it here and in gen_corpus.py DRIVE_LABEL", cell.Key, drive)
		}
		if recency := parts[3]; recency != "*" {
			_, ok := recencyLabel[recency]
			assert.True(t, ok, "decision table key %q uses recency %q with no entry in recencyLabel; add it here and in gen_corpus.py RECENCY_LABEL", cell.Key, recency)
		}
	}
}

// TestGoldenFileIsNotStale fails loudly if the golden was produced from a
// different template than the one compiled in. The corpus records the template
// hash it was built from; this checks the cheaper local half of that contract.
func TestGoldenFileIsNotStale(t *testing.T) {
	golden := readGolden(t, "golden_prompt_no_history.txt")

	for _, section := range []string{
		"FAKTA (tidak boleh diubah atau ditambah):",
		"KEPUTUSAN SISTEM (tidak boleh diubah):",
		"PERTANYAAN: ",
	} {
		assert.Contains(t, golden, section,
			"golden file no longer looks like prompt_template.txt (hash %s) -- regenerate it", TemplateHash[:12])
	}

	assert.NotContains(t, golden, "RIWAYAT:",
		"this golden is the no-history case; the RIWAYAT: header must have been dropped")
	assert.NotContains(t, golden, "\n\n\n", "blank runs should have been collapsed to one")
}
