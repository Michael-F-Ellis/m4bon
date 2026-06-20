package musicxml

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mellis/m4bon/parser"
)

var updateGolden = flag.Bool("update-golden", false, "Update golden (.expected.mxml) files with current output")

// TestGoldenFiles exercises the full pipeline (ParseDSL + Generate) on every
// .dsl test case. This is the in-process version of cmd/m4bon/golden_test.go
// — faster, debuggable, and works with breakpoints and -run.
func TestGoldenFiles(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(root, "test/cases/*.dsl"))
	if err != nil || len(matches) == 0 {
		t.Fatal("no .dsl test case files found")
	}

	for _, dslPath := range matches {
		name := filepath.Base(dslPath)
		if strings.HasPrefix(name, "render-") {
			continue // render test cases have their own golden test
		}
		base := strings.TrimSuffix(name, ".dsl")
		expectedPath := filepath.Join(root, "test/cases", base+".expected.mxml")

		isErrorCase := strings.HasPrefix(base, "error-")

		t.Run(base, func(t *testing.T) {
			dslData, err := os.ReadFile(dslPath)
			if err != nil {
				t.Fatalf("cannot read %s: %v", dslPath, err)
			}
			// Strip comment lines (first line often a comment)
			dsl := parser.SanitizeDSL(string(dslData))

			result := parser.ParseDSL(dsl)
			if isErrorCase {
				if result.Err == nil {
					t.Fatalf("expected error for error test case %s", name)
				}
				if !strings.Contains(result.Err.Error(), "Measure") {
					t.Errorf("error output for %s should contain measure info, got: %v", name, result.Err)
				}
				return
			}

			if result.Err != nil {
				t.Fatalf("ParseDSL failed for %s: %v", name, result.Err)
			}

			got, err := Generate(result.Measures, result.Key.Fifths)
			if err != nil {
				t.Fatalf("Generate failed for %s: %v", name, err)
			}
			got += "\n" // match CLI binary behavior (fmt.Println adds newline)

			if *updateGolden {
				if err := os.WriteFile(expectedPath, []byte(got), 0644); err != nil {
					t.Fatalf("failed to update golden file %s: %v", expectedPath, err)
				}
				t.Logf("updated %s", expectedPath)
				return
			}

			expected, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("cannot read expected file %s: %v\n\nTo create it, run:\n  go test ./musicxml -update-golden", expectedPath, err)
			}

			if string(expected) != got {
				t.Errorf("output mismatch for %s\n\nExpected:\n%s\n\nGot:\n%s", name, string(expected), got)
			}
		})
	}
}

// repoRoot finds the module root by walking up from the musicxml package.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
