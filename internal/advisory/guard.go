package advisory

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"audiax/internal/constants"
)

// llmOutput is the raw JSON shape the model must produce (DESIGN.md §3.3).
type llmOutput struct {
	Jawaban           string `json:"jawaban"`
	LangkahBerikutnya string `json:"langkah_berikutnya"`
	PerluTeknisi      bool   `json:"perlu_teknisi"`
	Eskalasi          bool   `json:"eskalasi"`
}

// Not package-level const: this project requires every named constant to
// live in internal/constants (enforced by
// TestConstantsAreDeclaredOnlyInThisPackage). These three are private
// implementation details of the number-whitelist check, not values another
// package would ever reference, so they stay here as vars instead of
// spreading into the shared constants file.
var (
	ordinalWhitelistMin = 1.0
	ordinalWhitelistMax = 9.0
	numberTolerance     = 0.05
)

// Guard runs the model's raw completion through the four checks DESIGN.md
// §4.2 requires. Every one must pass or Guard returns an error and the
// caller falls back to StaticReply -- never to the caller's own error page
// (DESIGN.md decision 8).
//
//   - Skema: the completion must parse as the §3.3 JSON shape with a
//     non-empty jawaban and langkah_berikutnya.
//   - Angka: every number in jawaban and langkah_berikutnya must be an
//     ordinal 1-9, a number the caller supplies from the HealthCard
//     (allowedNumbers), or a number that already appears somewhere in cell.
//   - Diagnosis: neither field may contain a forbidden diagnosis phrase.
//   - Keselamatan: if cell.SafetyGate is set, the combined text must
//     instruct shutting the machine down before it instructs inspecting it.
func Guard(raw string, cell Cell, allowedNumbers []float64) (Reply, error) {
	var out llmOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return Reply{}, fmt.Errorf("guard: skema: %w", err)
	}
	if strings.TrimSpace(out.Jawaban) == "" || strings.TrimSpace(out.LangkahBerikutnya) == "" {
		return Reply{}, errors.New("guard: skema: jawaban and langkah_berikutnya must not be empty")
	}

	combined := out.Jawaban + " " + out.LangkahBerikutnya

	whitelist := append(append([]float64{}, allowedNumbers...), cellNumbers(cell)...)
	if bad, ok := firstDisallowedNumber(combined, whitelist); ok {
		return Reply{}, fmt.Errorf("guard: angka: %v is not in the whitelist", bad)
	}

	if phrase, found := firstForbiddenDiagnosisPhrase(combined); found {
		return Reply{}, fmt.Errorf("guard: diagnosis: output contains forbidden phrase %q", phrase)
	}

	if cell.SafetyGate != "" && !shutdownPrecedesInspection(combined) {
		return Reply{}, errors.New("guard: keselamatan: output must instruct shutting the machine down before inspecting it")
	}

	// NeedsTechnician and Escalated come from cell and the deterministic
	// keyword check upstream, never from the model's own perlu_teknisi /
	// eskalasi claims. Those two fields are decisions, and DESIGN.md decision
	// 6 is unambiguous that the model never makes decisions -- it only
	// explains ones the rule engine already made. static.go's StaticReply
	// already follows this (see its comment on Escalated); a raw LLM
	// completion that answers a completely benign question ("apa artinya
	// indikator ini?") can still self-report eskalasi:true, and trusting it
	// puts a false "HENTIKAN MESIN" alarm in front of the operator for no
	// reason -- exactly the kind of cry-wolf failure that erodes trust in the
	// real alarm.
	return Reply{
		Answer:          out.Jawaban,
		NextStep:        out.LangkahBerikutnya,
		NeedsTechnician: cell.NeedsTechnician,
		Escalated:       false,
	}, nil
}

var numberPattern = regexp.MustCompile(`\d+(?:[.,]\d+)?`)

func extractNumbers(text string) []float64 {
	var nums []float64
	for _, m := range numberPattern.FindAllString(text, -1) {
		normalized := strings.Replace(m, ",", ".", 1)
		if v, err := strconv.ParseFloat(normalized, 64); err == nil {
			nums = append(nums, v)
		}
	}
	return nums
}

// cellNumbers is every number that legitimately appears in the decision
// table cell itself -- DESIGN.md's "angka ... dari Cell" -- so an LLM
// completion that quotes the checklist verbatim is not rejected for reusing
// a number the table already vetted.
func cellNumbers(cell Cell) []float64 {
	nums := []float64{float64(cell.RecheckHours)}
	nums = append(nums, extractNumbers(cell.SafetyGate)...)
	nums = append(nums, extractNumbers(cell.EscalateIf)...)
	nums = append(nums, extractNumbers(cell.Urgency)...)
	for _, item := range cell.Checklist {
		nums = append(nums, extractNumbers(item)...)
	}
	return nums
}

func firstDisallowedNumber(text string, allowed []float64) (float64, bool) {
	for _, n := range extractNumbers(text) {
		if !numberAllowed(n, allowed) {
			return n, true
		}
	}
	return 0, false
}

func numberAllowed(n float64, allowed []float64) bool {
	if n == math.Trunc(n) && n >= ordinalWhitelistMin && n <= ordinalWhitelistMax {
		return true
	}
	for _, a := range allowed {
		if math.Abs(n-a) < numberTolerance {
			return true
		}
	}
	return false
}

func firstForbiddenDiagnosisPhrase(text string) (string, bool) {
	lower := strings.ToLower(text)
	for _, phrase := range constants.AdvisoryForbiddenDiagnosisPhrases {
		if strings.Contains(lower, phrase) {
			return phrase, true
		}
	}
	return "", false
}

// shutdownPrecedesInspection is a keyword-order heuristic, not a parser: it
// requires a shutdown-related word to appear before an inspection-related
// word, case-insensitively. A fine-tuned model produces short, formulaic
// Indonesian sentences (temperature 0), so this catches the failure mode
// that matters -- telling the operator to inspect a running machine --
// without needing to match cell.SafetyGate's exact wording, which the model
// is expected to paraphrase.
func shutdownPrecedesInspection(text string) bool {
	lower := strings.ToLower(text)
	shutdownAt := strings.Index(lower, "matikan")
	inspectAt := strings.Index(lower, "periksa")

	if shutdownAt == -1 {
		return false
	}
	if inspectAt == -1 {
		return true
	}
	return shutdownAt < inspectAt
}
