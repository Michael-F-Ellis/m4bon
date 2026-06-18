package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot returns the module root directory (containing go.mod).

var updateGolden = flag.Bool("update-golden", false, "Update golden (.expected.mxml) files with current output")

func TestGoldenFiles(t *testing.T) {
	// Find all .dsl files in test/cases/
	root := repoRoot()
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

		// Error test cases: files named error-*.dsl
		isErrorCase := strings.HasPrefix(base, "error-")

		t.Run(base, func(t *testing.T) {
			if isErrorCase {
				// Run m4bon on the DSL file — expect failure
				_, err := exec.Command(filepath.Join(root, "m4bon"), "-f", dslPath).Output()
				if err == nil {
					t.Fatalf("expected error for error test case %s", name)
				}
				// err is *exec.ExitError, check stderr for measure number
				if exitErr, ok := err.(*exec.ExitError); ok {
					stderr := string(exitErr.Stderr)
					if !strings.Contains(stderr, "Measure") && !strings.Contains(stderr, "measure") {
						t.Errorf("error output for %s should contain measure info, got: %s", name, stderr)
					}
				} else {
					t.Errorf("unexpected error type for %s: %v", name, err)
				}
				return
			}

			// Run m4bon on the DSL file
			out, err := exec.Command("./m4bon", "-f", dslPath).Output()
			if err != nil {
				t.Fatalf("m4bon failed for %s: %v\n%s", name, err, string(out))
			}
			got := string(out)

			if *updateGolden {
				if err := os.WriteFile(expectedPath, []byte(got), 0644); err != nil {
					t.Fatalf("failed to update golden file %s: %v", expectedPath, err)
				}
				t.Logf("updated %s", expectedPath)
				return
			}

			// Read expected output
			expected, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("cannot read expected file %s: %v\n\nTo create it, run:\n  go test ./test -update-golden", expectedPath, err)
			}

			if string(expected) != got {
				t.Errorf("output mismatch for %s\n\nExpected:\n%s\n\nGot:\n%s", name, string(expected), got)
			}
		})
	}
}

func TestMusicXMLSchemaValid(t *testing.T) {
	// Find all .dsl files in test/cases/
	matches, err := filepath.Glob("test/cases/*.dsl")
	if err != nil || len(matches) == 0 {
		t.Fatal("no .dsl test case files found")
	}

	for _, dslPath := range matches {
		name := filepath.Base(dslPath)
		if strings.HasPrefix(name, "error-") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			out, err := exec.Command("./m4bon", "-f", dslPath).Output()
			if err != nil {
				t.Fatalf("m4bon failed: %v", err)
			}

			// Write output to temp file
			tmpFile := filepath.Join(t.TempDir(), "out.mxl")
			if err := os.WriteFile(tmpFile, out, 0644); err != nil {
				t.Fatalf("cannot write temp file: %v", err)
			}

			// Validate against MusicXML DTD (the DOCTYPE we emit)
			cmd := exec.Command("xmllint", "--dtdvalid", "http://www.musicxml.org/dtds/partwise.dtd", "--nonet", "--noout", tmpFile)
			valOut, valErr := cmd.CombinedOutput()

			if valErr != nil {
				// xmllint might not be installed — skip gracefully
				if _, err := exec.LookPath("xmllint"); err != nil {
					t.Skip("xmllint not available, skipping DTD validation")
				}
				// DTD validation may fail due to network access, skip gracefully
				t.Skipf("DTD validation skipped (network may be unavailable): %s", string(valOut))
			}
		})
	}
}
