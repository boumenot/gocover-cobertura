package main

import (
	"encoding/xml"
	"io"
	"io/ioutil"
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
	outputBytes, err := ioutil.ReadFile(fname)
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
	err := convert(strings.NewReader("invalid data"), pipe2wr, io.Discard, &Ignore{})
	require.Error(t, err)
	require.Equal(t, "bad mode line: invalid data", err.Error())
}

func TestConvertOutputError(t *testing.T) {
	pipe2rd, pipe2wr := io.Pipe()
	err := pipe2wr.Close()
	require.NoError(t, err)
	defer func() { err := pipe2rd.Close(); require.NoError(t, err) }()
	err = convert(strings.NewReader("mode: set"), pipe2wr, io.Discard, &Ignore{})
	require.Error(t, err)
	require.Equal(t, "io: read/write on closed pipe", err.Error())
}

func TestConvertEmpty(t *testing.T) {
	data := `mode: set`

	pipe2rd, pipe2wr := io.Pipe()
	go func() {
		err := convert(strings.NewReader(data), pipe2wr, io.Discard, &Ignore{})
		require.NoError(t, err)
	}()

	v := Coverage{}
	dec := xml.NewDecoder(pipe2rd)
	err := dec.Decode(&v)
	require.NoError(t, err)

	require.Equal(t, "coverage", v.XMLName.Local)
	require.Nil(t, v.Sources)
	require.Nil(t, v.Packages)
}

func TestParseProfileNilPackages(t *testing.T) {
	v := Coverage{}
	profile := Profile{FileName: "does-not-exist"}
	err := v.parseProfile(&profile, nil, &Ignore{})
	require.Error(t, err)
	require.Contains(t, `package required when using go modules`, err.Error())
}

func TestParseProfileEmptyPackages(t *testing.T) {
	v := Coverage{}
	profile := Profile{FileName: "does-not-exist"}
	err := v.parseProfile(&profile, &packages.Package{}, &Ignore{})
	require.Error(t, err)
	require.Contains(t, `package required when using go modules`, err.Error())
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

	// Windows vs. Linux
	if !strings.Contains(err.Error(), "system cannot find the file specified") &&
		!strings.Contains(err.Error(), "no such file or directory") {
		t.Errorf(err.Error())
	}
}

func TestParseProfileNotReadable(t *testing.T) {
	v := Coverage{}
	profile := Profile{FileName: os.DevNull}
	err := v.parseProfile(&profile, nil, &Ignore{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "package required when using go modules")
}

func TestParseProfilePermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is not supported by Windows")
	}

	tempFile, err := ioutil.TempFile("", "not-readable")
	require.NoError(t, err)

	defer func() { err := os.Remove(tempFile.Name()); require.NoError(t, err) }()
	err = tempFile.Chmod(000)
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
		err := convert(pipe1rd, convwr, io.Discard, &Ignore{
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
	if l = m.Lines[1]; l.Number != 5 || l.Hits != 0 {
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
	if l = c.Lines[1]; l.Number != 5 || l.Hits != 0 {
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
