package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var updateRender = flag.Bool("update-render", false, "Update golden (.expected.render) files with current output")

func TestRenderGoldenFiles(t *testing.T) {
	root := repoRoot()
	matches, err := filepath.Glob(filepath.Join(root, "test/cases/render-*.dsl"))
	if err != nil || len(matches) == 0 {
		t.Log("no render golden test case files found (test/cases/render-*.dsl)")
		return
	}

	for _, dslPath := range matches {
		name := filepath.Base(dslPath)
		base := strings.TrimSuffix(name, ".dsl")
		expectedPath := filepath.Join(root, "test/cases", base+".expected.render")

		t.Run(base, func(t *testing.T) {
			out, err := exec.Command(filepath.Join(root, "m4bon"), "-render", "-f", dslPath).Output()
			if err != nil {
				t.Fatalf("m4bon -render failed for %s: %v\n%s", name, err, string(out))
			}
			got := string(out)

			if *updateRender {
				if err := os.WriteFile(expectedPath, []byte(got), 0644); err != nil {
					t.Fatalf("failed to update golden file %s: %v", expectedPath, err)
				}
				t.Logf("updated %s", expectedPath)
				return
			}

			expected, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("cannot read expected file %s: %v\n\nTo create it, run:\n  go test -run TestRenderGoldenFiles -update-render", expectedPath, err)
			}

			if string(expected) != got {
				t.Errorf("output mismatch for %s\n\nExpected bytes:\n%q\n\nGot bytes:\n%q", name, string(expected), got)
			}
		})
	}
}
