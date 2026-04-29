package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "update regression golden files")

// modulePackages maps project directory names to their Go module paths.
var modulePackages = map[string]string{
	"mux": "github.com/gorilla/mux",
}

// regressionEntry describes a single regression test case discovered on disk.
type regressionEntry struct {
	Project      string
	Tag          string
	CoveragePath string
	GoldenPath   string
}

// discoverRegressionEntries walks root looking for coverage.txt files in the
// layout {root}/{project}/{tag}/coverage.txt and returns the corresponding
// entries. If root does not exist the function returns nil without error.
func discoverRegressionEntries(t *testing.T, root string) []regressionEntry {
	t.Helper()

	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}

	var entries []regressionEntry
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Name() != "coverage.txt" {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 3 {
			return nil
		}

		entries = append(entries, regressionEntry{
			Project:      parts[0],
			Tag:          parts[1],
			CoveragePath: path,
			GoldenPath:   filepath.Join(filepath.Dir(path), "golden.xml"),
		})
		return nil
	})
	require.NoError(t, err)
	return entries
}

// ensureModuleCached runs "go get" to make sure the module source for the
// given project and tag is present in the local module cache.
func ensureModuleCached(t *testing.T, project, tag string) {
	t.Helper()

	modPath, ok := modulePackages[project]
	if !ok {
		t.Fatalf("no module path configured for project %q", project)
	}

	cmd := exec.Command("go", "get", modPath+"@"+tag)
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go get %s@%s failed: %v\n%s", modPath, tag, err, out)
	}
}

var timestampRe = regexp.MustCompile(`timestamp="[0-9]+"`)
var sourceRe = regexp.MustCompile(`<source>[^<]*</source>`)

// normalizeOutput applies deterministic transformations to XML output so that
// golden comparisons are reproducible across machines and runs.
func normalizeOutput(s string) string {
	s = timestampRe.ReplaceAllString(s, `timestamp="0"`)
	s = sourceRe.ReplaceAllString(s, `<source>$GOMODCACHE</source>`)
	s = strings.ReplaceAll(s, `\`, `/`)

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return strings.Join(lines, "\n")
}

// runConvert feeds coverageData through convert() and returns the normalized
// XML output.
func runConvert(t *testing.T, coverageData string) string {
	t.Helper()

	var buf bytes.Buffer
	err := convert(strings.NewReader(coverageData), &buf, &Ignore{})
	require.NoError(t, err)
	return normalizeOutput(buf.String())
}

// unifiedDiff produces a human-readable line-by-line diff between expected and
// actual, showing at most 30 differences.
func unifiedDiff(expected, actual string) string {
	expLines := strings.Split(expected, "\n")
	actLines := strings.Split(actual, "\n")

	var diff strings.Builder
	const maxDiffs = 30
	shown := 0

	maxLen := len(expLines)
	if len(actLines) > maxLen {
		maxLen = len(actLines)
	}

	for i := 0; i < maxLen; i++ {
		if shown >= maxDiffs {
			fmt.Fprintf(&diff, "... (%d more differences omitted)\n", maxLen-i)
			break
		}
		var exp, act string
		if i < len(expLines) {
			exp = expLines[i]
		}
		if i < len(actLines) {
			act = actLines[i]
		}
		if exp != act {
			fmt.Fprintf(&diff, "line %d:\n  - %s\n  + %s\n", i+1, exp, act)
			shown++
		}
	}
	return diff.String()
}

// TestRegression discovers regression test cases under testdata/regression/
// and verifies that convert() output matches golden files. When -update is
// passed the golden files are written instead.
func TestRegression(t *testing.T) {
	root := filepath.Join("testdata", "regression")
	entries := discoverRegressionEntries(t, root)
	if len(entries) == 0 {
		t.Skip("no regression test data found under testdata/regression/")
	}

	for _, entry := range entries {
		name := entry.Project + "/" + entry.Tag
		t.Run(name, func(t *testing.T) {
			ensureModuleCached(t, entry.Project, entry.Tag)

			coverageData, err := os.ReadFile(entry.CoveragePath)
			require.NoError(t, err)

			actual := runConvert(t, string(coverageData))

			if *updateGolden {
				err := os.WriteFile(entry.GoldenPath, []byte(actual), 0o644)
				require.NoError(t, err)
				t.Logf("updated golden file: %s", entry.GoldenPath)
				return
			}

			goldenData, err := os.ReadFile(entry.GoldenPath)
			require.NoError(t, err, "golden file missing; run with -update to create")

			expected := normalizeOutput(string(goldenData))
			if expected != actual {
				t.Errorf("output differs from golden file %s:\n%s", entry.GoldenPath, unifiedDiff(expected, actual))
			}
		})
	}
}
