//go:build regression

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "update regression golden files")

// regressionEntry describes a single regression test case discovered on disk.
type regressionEntry struct {
	Project      string
	Tag          string
	CoveragePath string
	GoldenPath   string
}

// moduleRepos maps project directory names to their Git clone URLs.
var moduleRepos = map[string]string{
	"cobra":              "https://github.com/spf13/cobra",
	"fzf":               "https://github.com/junegunn/fzf",
	"go-approval-tests": "https://github.com/approvals/go-approval-tests",
	"mux":               "https://github.com/gorilla/mux",
	"prometheus":         "https://github.com/prometheus/prometheus",
	"testify":            "https://github.com/stretchr/testify",
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

// buildBinary compiles gocover-cobertura into a temporary binary and returns
// its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	name := "gocover-cobertura-test"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to build binary: %s", string(out))
	return bin
}

// ensureSourceAvailable clones the project repo at the given tag into a
// src/ subdirectory alongside the coverage.txt file. Reuses an existing
// clone if present. The src/ directories should be added to .gitignore.
func ensureSourceAvailable(t *testing.T, project, tag string) string {
	t.Helper()

	repoURL, ok := moduleRepos[project]
	if !ok {
		t.Fatalf("no repo URL configured for project %q", project)
	}

	cloneDir := filepath.Join("testdata", "regression", project, tag, "src")

	// Reuse existing clone if present.
	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err == nil {
		return cloneDir
	}

	cmd := exec.Command("git", "clone", "--depth", "1", "--branch", tag, repoURL, cloneDir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git clone failed: %s", string(out))

	return cloneDir
}

// runConvertSubprocess runs the gocover-cobertura binary as a subprocess
// with its working directory set to sourceDir (so packages.Load resolves
// correctly). Coverage data is piped to stdin.
func runConvertSubprocess(t *testing.T, binary, sourceDir, coverageData string) string {
	t.Helper()

	cmd := exec.Command(binary)
	cmd.Dir = sourceDir
	cmd.Stdin = strings.NewReader(coverageData)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.NoError(t, err, "gocover-cobertura failed (stderr: %s)", stderr.String())

	return normalizeOutput(stdout.String())
}

var timestampRe = regexp.MustCompile(`timestamp="[0-9]+"`)
var sourceRe = regexp.MustCompile(`<source>[^<]*</source>`)

// normalizeOutput applies deterministic transformations to XML output so that
// golden comparisons are reproducible across machines and runs.
func normalizeOutput(s string) string {
	s = timestampRe.ReplaceAllString(s, `timestamp="0"`)
	s = sourceRe.ReplaceAllLiteralString(s, `<source>$GOMODCACHE</source>`)
	s = strings.ReplaceAll(s, `\`, `/`)

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return strings.Join(lines, "\n")
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
			fmt.Fprintf(&diff, "... (%d more lines not checked)\n", maxLen-i)
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
// and verifies that gocover-cobertura output matches golden files. The binary
// is built once and run as a subprocess inside each project's cloned source
// tree so that packages.Load() can resolve packages correctly.
//
// When -update is passed, golden files are written instead of compared.
// Golden files are generated on Linux and may include platform-specific
// coverage. Skip on non-Linux to avoid false failures from missing
// platform-specific source files (e.g. _windows.go on Linux or vice versa).
func TestRegression(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("regression tests are Linux-only (golden files generated on Linux)")
	}

	root := filepath.Join("testdata", "regression")
	entries := discoverRegressionEntries(t, root)
	if len(entries) == 0 {
		t.Skip("no regression test data found under testdata/regression/")
	}

	binary := buildBinary(t)

	for _, entry := range entries {
		name := entry.Project + "/" + entry.Tag
		t.Run(name, func(t *testing.T) {
			sourceDir := ensureSourceAvailable(t, entry.Project, entry.Tag)

			coverageData, err := os.ReadFile(entry.CoveragePath)
			require.NoError(t, err)

			actual := runConvertSubprocess(t, binary, sourceDir, string(coverageData))

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
