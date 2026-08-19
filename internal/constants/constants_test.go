package constants_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const repoRoot = "../.."

// The project rule is that every constant lives in internal/constants. A rule
// nobody can enforce is a rule that decays, so this test walks the tree and
// fails on any `const` declared elsewhere.
func TestConstantsAreDeclaredOnlyInThisPackage(t *testing.T) {
	var offenders []string

	walkGoFiles(t, func(path string, file *ast.File, fset *token.FileSet) {
		if filepath.Dir(path) == filepath.Join(repoRoot, "internal", "constants") {
			return
		}

		ast.Inspect(file, func(node ast.Node) bool {
			decl, ok := node.(*ast.GenDecl)
			if !ok || decl.Tok != token.CONST {
				return true
			}
			for _, spec := range decl.Specs {
				for _, name := range spec.(*ast.ValueSpec).Names {
					offenders = append(offenders,
						fset.Position(name.Pos()).String()+": "+name.Name)
				}
			}
			return true
		})
	})

	assert.Empty(t, offenders,
		"constants must be declared in internal/constants, not inline")
}

// Everything imports this package, so it must import nothing from this module
// or it becomes an import cycle waiting to happen.
func TestConstantsPackageImportsNothingFromThisModule(t *testing.T) {
	var offenders []string

	walkGoFiles(t, func(path string, file *ast.File, _ *token.FileSet) {
		if filepath.Dir(path) != filepath.Join(repoRoot, "internal", "constants") {
			return
		}
		for _, imported := range file.Imports {
			if strings.HasPrefix(strings.Trim(imported.Path.Value, `"`), "audiax/") {
				offenders = append(offenders, imported.Path.Value)
			}
		}
	})

	assert.Empty(t, offenders, "internal/constants must stay a leaf package")
}

func walkGoFiles(t *testing.T, visit func(path string, file *ast.File, fset *token.FileSet)) {
	t.Helper()
	fset := token.NewFileSet()

	err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name := entry.Name(); name == ".git" || name == "bin" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		visit(path, file, fset)
		return nil
	})
	require.NoError(t, err)
}
