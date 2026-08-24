package advisory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"audiax/internal/constants"
)

func TestLookupReturnsNormalCellRegardlessOfOtherDimensions(t *testing.T) {
	cell, err := Lookup(constants.StatusNormal, "kurtosis", "belt", "<1bln")

	require.NoError(t, err)
	assert.Equal(t, "NORMAL|*|*|*", cell.Key)
	assert.Empty(t, cell.SafetyGate)
}

func TestLookupPrefersMostSpecificCellOverWildcard(t *testing.T) {
	specific, err := Lookup(constants.StatusWarning, "crest_factor", "belt", ">6bln")
	require.NoError(t, err)
	assert.Equal(t, "WARNING|crest_factor|belt|>6bln", specific.Key)

	general, err := Lookup(constants.StatusWarning, "crest_factor", "belt", "1-6bln")
	require.NoError(t, err)
	assert.Equal(t, "WARNING|crest_factor|belt|*", general.Key)

	assert.NotEqual(t, specific.RecheckHours, general.RecheckHours,
		"the overdue-maintenance override should differ from the default cell, or it has no reason to exist")
}

func TestLookupReturnsErrorForUnknownStatus(t *testing.T) {
	_, err := Lookup("SOMETHING_ELSE", "kurtosis", "belt", "<1bln")

	assert.Error(t, err)
}

func TestLookupCoversEveryCombinationExactlyOnceWithNoTies(t *testing.T) {
	statuses := []string{
		constants.StatusNormal,
		constants.StatusWarning,
		constants.StatusCritical,
		constants.StatusUncalibrated,
	}
	indicators := []string{"kurtosis", "crest_factor", "spectral_centroid", "tidak_ada"}
	drives := []string{"belt", "direct-coupled", "direct-drive"}
	recencies := []string{"<1bln", "1-6bln", ">6bln", "tidak-tahu"}

	seen := map[string]bool{}

	for _, status := range statuses {
		for _, indicator := range indicators {
			for _, drive := range drives {
				for _, recency := range recencies {
					candidates := matchingCells(status, indicator, drive, recency)
					require.NotEmptyf(t, candidates, "no cell matches %s|%s|%s|%s", status, indicator, drive, recency)

					best := leastWildcards(candidates)
					require.Lenf(t, best, 1,
						"%s|%s|%s|%s matches %d cells tied on wildcard count: %v",
						status, indicator, drive, recency, len(best), keysOf(best))

					seen[best[0].Key] = true
				}
			}
		}
	}

	for _, cell := range table.Cells {
		assert.Truef(t, seen[cell.Key], "cell %q is never the winning match for any combination (dead cell)", cell.Key)
	}
}

func keysOf(cells []Cell) []string {
	keys := make([]string, len(cells))
	for i, c := range cells {
		keys[i] = c.Key
	}
	return keys
}
