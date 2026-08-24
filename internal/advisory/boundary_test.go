package advisory

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allowedImportPrefixes is everything internal/advisory (and its tests) may
// import beyond the standard library: DESIGN.md §4.3 says this package must
// stay free of GORM, internal/entity, internal/config, internal/repository,
// internal/usecase, and any HTTP framework. internal/constants is the one
// project package it may use, since that package imports nothing from this
// module and so cannot drag any of the forbidden dependencies back in.
var allowedImportPrefixes = []string{
	"audiax/internal/constants",
	"github.com/stretchr/testify",
}

// TestPackageStaysWithinItsImportBoundary mirrors ai/tests/test_boundary.py
// in the AI service repo: the same architectural line is enforced by a test
// on both sides, not by convention alone.
func TestPackageStaysWithinItsImportBoundary(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	var offenders []string

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		file, err := parser.ParseFile(fset, entry.Name(), nil, parser.ImportsOnly)
		require.NoError(t, err)

		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if isAllowedImport(path) {
				continue
			}
			offenders = append(offenders, entry.Name()+": "+path)
		}
	}

	assert.Empty(t, offenders, "internal/advisory must stay a pure package: stdlib + internal/constants only")
}

func isAllowedImport(path string) bool {
	if isStdlib(path) {
		return true
	}
	for _, prefix := range allowedImportPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// isStdlib treats a path as standard library when its first segment has no
// dot -- every module path (github.com/..., gorm.io/..., audiax/...) has one.
func isStdlib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}
