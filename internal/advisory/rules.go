// Package advisory is Teknisi Saku's rule engine and prompt/guard layer for
// AUDIAX. It is a pure package: it never imports GORM, internal/entity,
// internal/config, internal/repository, internal/usecase, or an HTTP
// framework (boundary_test.go enforces this). It accepts plain structs and
// returns plain structs; callers own every I/O concern.
//
// The LLM never decides anything here. decision_table.json is the single
// source of deterministic triage decisions; everything downstream only
// renders or validates around it.
package advisory

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Cell is one row of decision_table.json: the deterministic triage decision
// for a status|dominant_indicator|drive_type|recency combination. See
// DESIGN.md §3.1 for the schema and the wildcard-matching rule.
type Cell struct {
	Key             string   `json:"key"`
	Urgency         string   `json:"urgency"`
	SafetyGate      string   `json:"safety_gate"`
	Checklist       []string `json:"checklist"`
	EscalateIf      string   `json:"escalate_if"`
	RecheckHours    int      `json:"recheck_hours"`
	NeedsTechnician bool     `json:"needs_technician"`
}

type decisionTable struct {
	SchemaVersion string `json:"schema_version"`
	Cells         []Cell `json:"cells"`
}

//go:embed decision_table.json
var decisionTableJSON []byte

// table is parsed once at package init. A malformed embedded file is a build
// defect, not a runtime condition callers can recover from, so this panics
// rather than surfacing an error from every call site.
var table decisionTable

func init() {
	if err := json.Unmarshal(decisionTableJSON, &table); err != nil {
		panic("advisory: decision_table.json is not valid JSON: " + err.Error())
	}
}

// Lookup finds the decision Cell for a status|dominant_indicator|drive_type|
// recency combination, preferring the matching cell with the fewest
// wildcards. It returns an error only if decision_table.json does not cover
// the combination at all, which is a defect in the table, not an expected
// runtime path.
func Lookup(status, dominantIndicator, driveType, recency string) (Cell, error) {
	candidates := matchingCells(status, dominantIndicator, driveType, recency)
	if len(candidates) == 0 {
		return Cell{}, fmt.Errorf("advisory: no decision_table.json cell matches %s|%s|%s|%s",
			status, dominantIndicator, driveType, recency)
	}

	best := leastWildcards(candidates)
	return best[0], nil
}

// matchingCells returns every cell whose key matches the given combination,
// wildcards included.
func matchingCells(status, dominantIndicator, driveType, recency string) []Cell {
	// 4 dimensions: status|dominant_indicator|drive_type|recency (DESIGN.md §3.1).
	combo := [4]string{status, dominantIndicator, driveType, recency}

	var matches []Cell
	for _, cell := range table.Cells {
		if cellMatches(cell.Key, combo) {
			matches = append(matches, cell)
		}
	}
	return matches
}

func cellMatches(key string, combo [4]string) bool {
	parts := strings.Split(key, "|")
	if len(parts) != 4 {
		return false
	}
	for i, part := range parts {
		if part != "*" && part != combo[i] {
			return false
		}
	}
	return true
}

// leastWildcards returns the subset of cells with the fewest "*" segments in
// their key. A correct table always narrows this to exactly one cell; more
// than one is a bug in decision_table.json (see rules_test.go).
func leastWildcards(cells []Cell) []Cell {
	best := wildcardCount(cells[0].Key)
	for _, c := range cells[1:] {
		if w := wildcardCount(c.Key); w < best {
			best = w
		}
	}

	var out []Cell
	for _, c := range cells {
		if wildcardCount(c.Key) == best {
			out = append(out, c)
		}
	}
	return out
}

func wildcardCount(key string) int {
	return strings.Count(key, "*")
}
