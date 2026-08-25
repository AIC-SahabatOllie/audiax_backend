package advisory

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"audiax/internal/constants"
)

//go:embed prompt_template.txt
var promptTemplate []byte

// TemplateHash is the SHA-256 hex digest of prompt_template.txt -- the
// anti-skew check PROMPT_CONTRACT.md requires. Track B's training corpus
// records the hash of the template it was built from in corpus/meta.json;
// the evaluation harness must fail hard, not just warn, if that no longer
// matches this value. Exposed on /healthz in the delivery layer.
var TemplateHash = fmt.Sprintf("%x", sha256.Sum256(promptTemplate))

// Turn is one exchange already in the conversation, in the client's own
// vocabulary (DESIGN.md §3.4): Role is "user" (the operator) or "assistant"
// (this feature's own prior reply).
type Turn struct {
	Role    string
	Content string
}

// PromptInput is everything RenderPrompt needs to fill in prompt_template.txt.
// Every field is a fact the caller already owns -- this package never reaches
// into a database or an HTTP request itself.
type PromptInput struct {
	Status             string
	Z                  *float64
	ZWarningThreshold  float64
	ZCriticalThreshold float64
	DominantIndicator  string
	CalibrationQuality string
	DriveType          string
	Recency            string
	MachineAge         string
	HoursPerDay        string
	HasBackup          bool
	LoadState          string

	Cell Cell

	History     []Turn
	UserMessage string
}

var placeholderPattern = regexp.MustCompile(`\{(\w+)\}`)

// driveLabel, recencyLabel and ageLabel humanise the enum codes that arrive
// from the decision table and the database before they reach the prompt.
//
// These are not cosmetic. The training corpus was generated with exactly these
// substitutions (gen_corpus.py: DRIVE_LABEL / RECENCY_LABEL / AGE_LABEL), so a
// model that saw "penggerak: sabuk-puli" throughout training would meet an
// unseen token if production sent the raw code "belt". That is train/serve skew
// of the worst kind: nothing errors, the output just quietly degrades. Any new
// code added to decision_table.json must be added here AND to the Python map in
// the same commit.
//
// Urgency is deliberately NOT humanised on either side -- the corpus carries
// "urgensi: rencanakan_dalam_48_jam" verbatim from the decision table.
var (
	driveLabel = map[string]string{
		"belt":           "sabuk-puli",
		"direct-coupled": "kopling langsung",
		"direct-drive":   "direct-drive",
	}
	recencyLabel = map[string]string{
		"<1bln":      "<1 bulan",
		"1-6bln":     "1-6 bulan",
		">6bln":      ">6 bulan",
		"tidak-tahu": "tidak tahu",
	}
	ageLabel = map[string]string{
		"<1th":  "<1 tahun",
		"1-3th": "1-3 tahun",
		"3-5th": "3-5 tahun",
		">5th":  ">5 tahun",
	}
)

// humanise maps a code to its corpus-facing label, passing unknown values
// through unchanged. Pass-through rather than panic: an unrecognised code is a
// data problem that should degrade one field, not take down a request. The
// tests assert every code the decision table actually uses is covered, so a
// gap fails in CI rather than in front of an operator.
func humanise(labels map[string]string, code string) string {
	if label, ok := labels[code]; ok {
		return label
	}
	return code
}

// omittableIfEmpty is PROMPT_CONTRACT.md rule 2: a line whose only nullable
// placeholder is empty is dropped entirely, never written with "null" or left
// blank.
var omittableIfEmpty = map[string]bool{
	"z":                   true,
	"dominant_indicator":  true,
	"calibration_quality": true,
}

// RenderPrompt renders prompt_template.txt per PROMPT_CONTRACT.md. This is
// the only place in Go that produces this format; the notebook that builds
// Track B's training corpus renders the same embedded template file under
// the same rules. A change to either the template or these rules that is not
// mirrored on both sides is a silent train/serve skew, not a compile error.
func RenderPrompt(in PromptInput) string {
	values := scalarValues(in)
	templateLines := strings.Split(strings.TrimSuffix(string(promptTemplate), "\n"), "\n")

	var out []string
	for _, line := range templateLines {
		switch strings.TrimSpace(line) {
		case "{checklist}":
			if len(in.Cell.Checklist) == 0 {
				out = dropPrecedingHeader(out, "langkah:")
				continue
			}
			out = append(out, renderChecklist(in.Cell.Checklist)...)
			continue
		case "{history}":
			turns := cappedHistory(in.History)
			if len(turns) == 0 {
				out = dropPrecedingHeader(out, "RIWAYAT:")
				continue
			}
			out = append(out, renderHistory(turns)...)
			continue
		}

		if droppedByEmptyPlaceholder(line, values) {
			continue
		}
		out = append(out, substitute(line, values))
	}

	return strings.Join(collapseBlankRuns(out), "\n") + "\n"
}

func scalarValues(in PromptInput) map[string]string {
	return map[string]string{
		"status":              in.Status,
		"z":                   formatZ(in.Z),
		"z_warning":           formatFloat1(in.ZWarningThreshold),
		"z_critical":          formatFloat1(in.ZCriticalThreshold),
		"dominant_indicator":  in.DominantIndicator,
		"calibration_quality": in.CalibrationQuality,
		"drive_type":          humanise(driveLabel, in.DriveType),
		"recency":             humanise(recencyLabel, in.Recency),
		"machine_age":         humanise(ageLabel, in.MachineAge),
		"hours_per_day":       in.HoursPerDay,
		"has_backup":          formatYaTidak(in.HasBackup),
		"load_state":          in.LoadState,
		"urgency":             in.Cell.Urgency,
		"safety_gate":         in.Cell.SafetyGate,
		"escalate_if":         in.Cell.EscalateIf,
		"recheck_hours":       strconv.Itoa(in.Cell.RecheckHours),
		"user_message":        in.UserMessage,
	}
}

func droppedByEmptyPlaceholder(line string, values map[string]string) bool {
	for _, m := range placeholderPattern.FindAllStringSubmatch(line, -1) {
		name := m[1]
		if omittableIfEmpty[name] && values[name] == "" {
			return true
		}
	}
	return false
}

func substitute(line string, values map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(line, func(token string) string {
		return values[token[1:len(token)-1]]
	})
}

// dropPrecedingHeader removes the just-emitted line if it is exactly header
// (ignoring surrounding whitespace) -- used when a section's body is empty
// and its header should not dangle above nothing.
func dropPrecedingHeader(out []string, header string) []string {
	if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == header {
		return out[:len(out)-1]
	}
	return out
}

func renderChecklist(items []string) []string {
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = fmt.Sprintf("    %d. %s", i+1, item)
	}
	return lines
}

func cappedHistory(history []Turn) []Turn {
	if len(history) <= constants.AdvisoryMaxHistoryTurns {
		return history
	}
	return history[len(history)-constants.AdvisoryMaxHistoryTurns:]
}

func renderHistory(turns []Turn) []string {
	lines := make([]string, len(turns))
	for i, turn := range turns {
		prefix := "asisten"
		if turn.Role == "user" {
			prefix = "operator"
		}
		lines[i] = fmt.Sprintf("  %s: %s", prefix, turn.Content)
	}
	return lines
}

func formatZ(z *float64) string {
	if z == nil {
		return ""
	}
	return formatFloat1(*z)
}

func formatFloat1(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func formatYaTidak(v bool) string {
	if v {
		return "ya"
	}
	return "tidak"
}

// collapseBlankRuns merges consecutive blank lines into one. It only ever
// fires where a whole section (RIWAYAT: + its body) was dropped between two
// blank separator lines; the template itself never contains a blank run.
func collapseBlankRuns(lines []string) []string {
	var out []string
	prevBlank := false
	for _, l := range lines {
		blank := strings.TrimSpace(l) == ""
		if blank && prevBlank {
			continue
		}
		out = append(out, l)
		prevBlank = blank
	}
	return out
}
