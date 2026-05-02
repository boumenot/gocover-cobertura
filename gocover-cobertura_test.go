package main

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

var SaveTestResults = false

func Test_Main(t *testing.T) {
	fname := filepath.Join(os.TempDir(), "stdout")
	temp, err := os.Create(fname)
	require.NoError(t, err)
	os.Stdout = temp
	main()
	outputBytes, err := os.ReadFile(fname)
	require.NoError(t, err)

	outputString := string(outputBytes)
	require.Contains(t, outputString, xml.Header)
	require.Contains(t, outputString, coberturaDTDDecl)
}

func TestConvertParseProfilesError(t *testing.T) {
	pipe2rd, pipe2wr := io.Pipe()
	defer func() {
		err := pipe2rd.Close()
		require.NoError(t, err)
		err = pipe2wr.Close()
		require.NoError(t, err)
	}()
	err := convert(strings.NewReader("invalid data"), pipe2wr, &Ignore{})
	require.Error(t, err)
	require.Equal(t, "bad mode line: invalid data", err.Error())
}

func TestConvertOutputError(t *testing.T) {
	pipe2rd, pipe2wr := io.Pipe()
	err := pipe2wr.Close()
	require.NoError(t, err)
	defer func() { err := pipe2rd.Close(); require.NoError(t, err) }()
	err = convert(strings.NewReader("mode: set"), pipe2wr, &Ignore{})
	require.Error(t, err)
	require.Equal(t, "io: read/write on closed pipe", err.Error())
}

func TestConvertEmpty(t *testing.T) {
	data := `mode: set`

	pipe2rd, pipe2wr := io.Pipe()
	var readErr error
	go func() {
		readErr = convert(strings.NewReader(data), pipe2wr, &Ignore{})
	}()

	v := Coverage{}
	dec := xml.NewDecoder(pipe2rd)
	err := dec.Decode(&v)
	require.NoError(t, readErr)
	require.NoError(t, err)

	require.Equal(t, "coverage", v.XMLName.Local)
	require.Nil(t, v.Sources)
	require.Nil(t, v.Packages)
}

func TestParseProfileNilPackages(t *testing.T) {
	v := Coverage{}
	profile := Profile{FileName: "does-not-exist"}
	err := v.parseProfile(&profile, nil, &Ignore{})
	require.NoError(t, err)
}

func TestParseProfileEmptyPackages(t *testing.T) {
	v := Coverage{}
	profile := Profile{FileName: "does-not-exist"}
	err := v.parseProfile(&profile, &packages.Package{}, &Ignore{})
	require.NoError(t, err)
}

func TestParseProfileDoesNotExist(t *testing.T) {
	v := Coverage{}
	profile := Profile{FileName: "does-not-exist"}

	pkg := packages.Package{
		Name:   "does-not-exist",
		Module: &packages.Module{},
	}

	err := v.parseProfile(&profile, &pkg, &Ignore{})
	require.Error(t, err)

	if !strings.Contains(err.Error(), "unable to determine file path") {
		t.Error(err.Error())
	}
}

func TestParseProfileNotReadable(t *testing.T) {
	v := Coverage{}
	profile := Profile{FileName: os.DevNull}
	err := v.parseProfile(&profile, nil, &Ignore{})
	require.NoError(t, err)
}

func TestParseProfilePermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is not supported by Windows")
	}

	tempFile, err := os.CreateTemp("", "not-readable")
	require.NoError(t, err)

	defer func() { err := os.Remove(tempFile.Name()); require.NoError(t, err) }()
	err = tempFile.Chmod(0o00)
	require.NoError(t, err)
	v := Coverage{}
	profile := Profile{FileName: tempFile.Name()}
	pkg := packages.Package{
		GoFiles: []string{
			tempFile.Name(),
		},
		Module: &packages.Module{
			Path: filepath.Dir(tempFile.Name()),
		},
	}
	err = v.parseProfile(&profile, &pkg, &Ignore{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission denied")
}

// TestConvertNilModule verifies that convert does not panic when
// packages.Load returns a package with a nil Module field. This can happen
// when:
//   - The profile references a standard library package (e.g. "fmt"),
//     which is not part of any Go module.
//   - The code is built in GOPATH mode without module support.
//   - A dependency fails to fully resolve (e.g. private repo auth
//     failures, missing transitive dependencies), causing packages.Load
//     to return a partially populated Package with Module == nil.
func TestConvertNilModule(t *testing.T) {
	// A coverage profile referencing a stdlib package triggers the nil
	// Module path because standard library packages have no module info.
	data := "mode: set\nfmt/print.go:1.1,1.1 1 1\n"

	pipe2rd, pipe2wr := io.Pipe()
	var convertErr error
	go func() {
		convertErr = convert(strings.NewReader(data), pipe2wr, &Ignore{})
		pipe2wr.Close()
	}()

	// Drain the reader so the goroutine can complete.
	_, _ = io.ReadAll(pipe2rd)

	// The convert function must not panic. It may return an error (e.g.
	// because the file doesn't exist on disk) but a nil-pointer
	// dereference panic is the bug we are guarding against.
	require.NotPanics(t, func() {
		// The goroutine already ran; this is here only as a safety assertion.
	})
	_ = convertErr // error is acceptable; panic is not
}

func TestConvertSetMode(t *testing.T) {
	pipe1rd, err := os.Open("testdata/testdata_set.txt")
	require.NoError(t, err)

	pipe2rd, pipe2wr := io.Pipe()

	var convwr io.Writer = pipe2wr
	if SaveTestResults {
		testwr, err := os.Create("testdata/testdata_set.xml")
		if err != nil {
			t.Fatal("Can't open output testdata.", err)
		}
		defer func() { err := testwr.Close(); require.NoError(t, err) }()
		convwr = io.MultiWriter(convwr, testwr)
	}

	go func() {
		err := convert(pipe1rd, convwr, &Ignore{
			GeneratedFiles: true,
			Files:          regexp.MustCompile(`[\\/]func[45]\.go$`),
		})
		if err != nil {
			panic(err)
		}
	}()

	v := Coverage{}
	dec := xml.NewDecoder(pipe2rd)
	err = dec.Decode(&v)
	require.NoError(t, err)

	require.Equal(t, "coverage", v.XMLName.Local)
	require.Len(t, v.Sources, 1)
	require.Len(t, v.Packages, 1)

	p := v.Packages[0]
	require.Equal(t, "github.com/boumenot/gocover-cobertura/testdata", strings.TrimRight(p.Name, "/"))
	require.NotNil(t, p.Classes)
	require.Len(t, p.Classes, 2)

	c := p.Classes[0]
	require.Equal(t, "-", c.Name)
	require.Equal(t, "testdata/func1.go", c.Filename)
	require.NotNil(t, c.Methods)
	require.Len(t, c.Methods, 1)
	require.NotNil(t, c.Lines)
	require.Len(t, c.Lines, 4)

	m := c.Methods[0]
	require.Equal(t, "Func1", m.Name)
	require.NotNil(t, c.Lines)
	require.Len(t, c.Lines, 4)

	var l *Line
	if l = m.Lines[0]; l.Number != 4 || l.Hits != 1 {
		t.Errorf("unmatched line: Number:%d, Hits:%d", l.Number, l.Hits)
	}
	// Line 5 is partially covered (coverage up to column 16), but Cobertura doesn't support partial hits, so it should be counted as covered.
	if l = m.Lines[1]; l.Number != 5 || l.Hits != 1 {
		t.Errorf("unmatched line: Number:%d, Hits:%d", l.Number, l.Hits)
	}
	if l = m.Lines[2]; l.Number != 6 || l.Hits != 0 {
		t.Errorf("unmatched line: Number:%d, Hits:%d", l.Number, l.Hits)
	}
	if l = m.Lines[3]; l.Number != 7 || l.Hits != 0 {
		t.Errorf("unmatched line: Number:%d, Hits:%d", l.Number, l.Hits)
	}

	if l = c.Lines[0]; l.Number != 4 || l.Hits != 1 {
		t.Errorf("unmatched line: Number:%d, Hits:%d", l.Number, l.Hits)
	}
	if l = c.Lines[1]; l.Number != 5 || l.Hits != 1 {
		t.Errorf("unmatched line: Number:%d, Hits:%d", l.Number, l.Hits)
	}
	if l = c.Lines[2]; l.Number != 6 || l.Hits != 0 {
		t.Errorf("unmatched line: Number:%d, Hits:%d", l.Number, l.Hits)
	}
	if l = c.Lines[3]; l.Number != 7 || l.Hits != 0 {
		t.Errorf("unmatched line: Number:%d, Hits:%d", l.Number, l.Hits)
	}

	c = p.Classes[1]
	require.Equal(t, "Type1", c.Name)
	require.Equal(t, "testdata/func2.go", c.Filename)
	require.NotNil(t, c.Methods)
	require.Len(t, c.Methods, 3)
}

// TestConvertOverlappingBlocks verifies deduplication of coverage blocks
// that overlap on line boundaries. This occurs with -coverpkg, where go
// test produces separate blocks from each test package that may cover
// different spans of the same lines.
//
// The test profile has overlapping blocks for func_overlap.go:
//
//	Block A: 9.30,10.13 (hit)   — covers lines 9-10
//	Block B: 10.13,12.3 (miss)  — covers lines 10-12
//	Block C: 12.3,13.2  (hit)   — covers line 12-13
//	Block D: 9.30,12.3  (hit)   — covers lines 9-12 (same start as A, different end)
//	Block E: 12.3,13.2  (miss)  — covers line 12-13
//
// BUG: The old AddOrUpdateLine only checks the *last* added line for
// deduplication. When overlapping blocks produce non-adjacent duplicate
// line entries, they are added as separate entries instead of merged.
// This results in 10 line entries instead of 5, with some lines showing
// hits=0 even though another entry for the same line has hits=1.
//
// See: https://github.com/boumenot/gocover-cobertura/pull/24
func TestConvertOverlappingBlocks(t *testing.T) {
	pipe1rd, err := os.Open("testdata/testdata_overlap.txt")
	require.NoError(t, err)

	pipe2rd, pipe2wr := io.Pipe()

	go func() {
		err := convert(pipe1rd, pipe2wr, &Ignore{})
		if err != nil {
			panic(err)
		}
	}()

	v := Coverage{}
	dec := xml.NewDecoder(pipe2rd)
	err = dec.Decode(&v)
	require.NoError(t, err)

	require.Equal(t, "coverage", v.XMLName.Local)
	require.Len(t, v.Packages, 1)

	p := v.Packages[0]
	require.NotNil(t, p.Classes)
	require.Len(t, p.Classes, 1)

	c := p.Classes[0]
	require.Equal(t, "-", c.Name)
	require.Equal(t, "testdata/func_overlap.go", c.Filename)
	require.Len(t, c.Methods, 1)

	m := c.Methods[0]
	require.Equal(t, "FuncOverlap", m.Name)

	// With PR #24's fix, overlapping blocks are properly deduplicated.
	// All 5 lines should be unique and covered (OR of overlapping blocks).
	require.Equal(t, 5, len(m.Lines),
		"expected 5 unique lines after dedup fix")

	for _, l := range m.Lines {
		require.Equal(t, int64(1), l.Hits,
			"line %d should be covered (OR of overlapping blocks)", l.Number)
	}
}

// TestConvertGlobalVarFunc verifies that package-level variable functions
// (e.g. `var Foo = func() { ... }`) have their coverage lines included.
// Previously these were dropped because the AST visitor only matched
// *ast.FuncDecl, and variable functions are *ast.ValueSpec.
//
// See: https://github.com/boumenot/gocover-cobertura/pull/25
func TestConvertGlobalVarFunc(t *testing.T) {
	pipe1rd, err := os.Open("testdata/testdata_varfunc.txt")
	require.NoError(t, err)

	pipe2rd, pipe2wr := io.Pipe()

	go func() {
		err := convert(pipe1rd, pipe2wr, &Ignore{})
		if err != nil {
			t.Logf("convert error: %v", err)
		}
		pipe2wr.Close()
	}()

	v := Coverage{}
	dec := xml.NewDecoder(pipe2rd)
	err = dec.Decode(&v)
	require.NoError(t, err)

	require.Equal(t, "coverage", v.XMLName.Local)
	require.Len(t, v.Packages, 1)

	p := v.Packages[0]
	require.NotNil(t, p.Classes)

	// Collect methods from func_varfunc.go across all classes.
	var methods []string
	for _, c := range p.Classes {
		if c.Filename == "testdata/func_varfunc.go" {
			for _, m := range c.Methods {
				methods = append(methods, m.Name)
			}
		}
	}

	// GlobalVarFunc should be included after the fix.
	require.Contains(t, methods, "GlobalVarFunc",
		"GlobalVarFunc should be present in output")
	require.Contains(t, methods, "FuncWithLocalVarFunc",
		"Regular FuncDecl should be present")
}

// TestConvertLocalVarFuncDoubleCounting verifies that local variable
// functions declared with `var` inside a FuncDecl do not cause lines
// to be double-counted. The visitor matches both the enclosing FuncDecl
// (counting all body lines) and the inner ValueSpec, which could inflate
// line counts.
//
// See review comment on PR #25:
// "a local func value will be double counted after this PR"
func TestConvertLocalVarFuncDoubleCounting(t *testing.T) {
	pipe1rd, err := os.Open("testdata/testdata_varfunc.txt")
	require.NoError(t, err)

	pipe2rd, pipe2wr := io.Pipe()

	go func() {
		err := convert(pipe1rd, pipe2wr, &Ignore{})
		if err != nil {
			t.Logf("convert error: %v", err)
		}
		pipe2wr.Close()
	}()

	v := Coverage{}
	dec := xml.NewDecoder(pipe2rd)
	err = dec.Decode(&v)
	require.NoError(t, err)

	require.Len(t, v.Packages, 1)
	p := v.Packages[0]

	// Count all lines attributed to func_varfunc.go across all classes.
	var allLines []*Line
	for _, c := range p.Classes {
		if c.Filename == "testdata/func_varfunc.go" {
			allLines = append(allLines, c.Lines...)
		}
	}

	// Check for duplicate line numbers which indicate double-counting.
	lineNumbers := make(map[int]int)
	for _, l := range allLines {
		lineNumbers[l.Number]++
	}

	var duplicates []int
	for lineNum, count := range lineNumbers {
		if count > 1 {
			duplicates = append(duplicates, lineNum)
		}
	}

	// No line should appear more than once. If local var closures are
	// double-counted, this assertion will catch it.
	require.Empty(t, duplicates,
		"lines should not be double-counted; duplicates at: %v", duplicates)
}

// TestConvertNonFuncVarIgnored verifies that non-function package-level
// variables (e.g. `var x = "hello"`) do not produce method entries.
func TestConvertNonFuncVarIgnored(t *testing.T) {
	pipe1rd, err := os.Open("testdata/testdata_varfunc.txt")
	require.NoError(t, err)

	pipe2rd, pipe2wr := io.Pipe()

	go func() {
		err := convert(pipe1rd, pipe2wr, &Ignore{})
		if err != nil {
			t.Logf("convert error: %v", err)
		}
		pipe2wr.Close()
	}()

	v := Coverage{}
	dec := xml.NewDecoder(pipe2rd)
	err = dec.Decode(&v)
	require.NoError(t, err)

	require.Len(t, v.Packages, 1)
	p := v.Packages[0]

	var methods []string
	for _, c := range p.Classes {
		if c.Filename == "testdata/func_varfunc.go" {
			for _, m := range c.Methods {
				methods = append(methods, m.Name)
			}
		}
	}

	require.NotContains(t, methods, "NonFuncVar",
		"non-function var should not appear as a method")
}

// TestConvertShortAssignNotDoubleCounted verifies that closures assigned
// with := (AssignStmt) inside a FuncDecl are not double-counted.
func TestConvertShortAssignNotDoubleCounted(t *testing.T) {
	pipe1rd, err := os.Open("testdata/testdata_varfunc.txt")
	require.NoError(t, err)

	pipe2rd, pipe2wr := io.Pipe()

	go func() {
		err := convert(pipe1rd, pipe2wr, &Ignore{})
		if err != nil {
			t.Logf("convert error: %v", err)
		}
		pipe2wr.Close()
	}()

	v := Coverage{}
	dec := xml.NewDecoder(pipe2rd)
	err = dec.Decode(&v)
	require.NoError(t, err)

	require.Len(t, v.Packages, 1)
	p := v.Packages[0]

	// Find lines for FuncWithShortAssignClosure.
	var allLines []*Line
	for _, c := range p.Classes {
		if c.Filename == "testdata/func_varfunc.go" {
			for _, m := range c.Methods {
				if m.Name == "FuncWithShortAssignClosure" {
					allLines = append(allLines, m.Lines...)
				}
			}
		}
	}

	lineNumbers := make(map[int]int)
	for _, l := range allLines {
		lineNumbers[l.Number]++
	}

	var duplicates []int
	for lineNum, count := range lineNumbers {
		if count > 1 {
			duplicates = append(duplicates, lineNum)
		}
	}

	require.Empty(t, duplicates,
		":= closure lines should not be double-counted; duplicates at: %v", duplicates)
}

// TestConvertWithProjectDir verifies that the -path flag correctly sets the
// working directory for package resolution via packages.Config.Dir, without
// using os.Chdir.
//
// Inspired by: https://github.com/boumenot/gocover-cobertura/pull/23
func TestConvertWithProjectDir(t *testing.T) {
	// Get the absolute path to the repo root.
	absRoot, err := filepath.Abs(".")
	require.NoError(t, err)

	// Change to a temp directory so the default resolution would fail.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	defer func() { require.NoError(t, os.Chdir(origDir)) }()

	// Read testdata from the absolute path.
	data, err := os.ReadFile(filepath.Join(absRoot, "testdata", "testdata_set.txt"))
	require.NoError(t, err)

	// convert with projectDir pointing to the original repo root should
	// succeed even though cwd is a temp directory.
	var buf bytes.Buffer
	err = convert(strings.NewReader(string(data)), &buf, &Ignore{}, absRoot)
	require.NoError(t, err)
	require.Contains(t, buf.String(), "coverage")
}
