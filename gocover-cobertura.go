package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
)

const coberturaDTDDecl = `<!DOCTYPE coverage SYSTEM "http://cobertura.sourceforge.net/xml/coverage-04.dtd">`

var byFiles bool
var projectDir string
var strict bool

func fatal(format string, a ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format, a...)
	os.Exit(1)
}

func main() {
	var ignore Ignore

	flag.BoolVar(&byFiles, "by-files", false, "code coverage by file, not class")
	flag.BoolVar(&ignore.GeneratedFiles, "ignore-gen-files", false, "ignore generated files")
	flag.BoolVar(&ignore.NonCodeLines, "ignore-non-code-lines", false, "ignore lines with no code")
	flag.BoolVar(&strict, "strict", false, "exit with error if any profiles are skipped")
	flag.StringVar(&projectDir, "path", "", "set the project directory for package resolution")
	ignoreDirsRe := flag.String("ignore-dirs", "", "ignore dirs matching this regexp")
	ignoreFilesRe := flag.String("ignore-files", "", "ignore files matching this regexp")

	flag.Parse()

	var err error
	if *ignoreDirsRe != "" {
		ignore.Dirs, err = regexp.Compile(*ignoreDirsRe)
		if err != nil {
			fatal("Bad -ignore-dirs regexp: %s\n", err)
		}
	}

	if *ignoreFilesRe != "" {
		ignore.Files, err = regexp.Compile(*ignoreFilesRe)
		if err != nil {
			fatal("Bad -ignore-files regexp: %s\n", err)
		}
	}

	if strict {
		if err := convertStrict(os.Stdin, os.Stdout, &ignore, projectDir); err != nil {
			fatal("code coverage conversion failed: %s\n", err)
		}
	} else {
		if err := convert(os.Stdin, os.Stdout, &ignore, projectDir); err != nil {
			fatal("code coverage conversion failed: %s\n", err)
		}
	}
}

func convert(in io.Reader, out io.Writer, ignore *Ignore, pkgDir ...string) error {
	_, err := convertInternal(in, out, ignore, pkgDir...)
	return err
}

// convertStrict is like convert but returns an error if any profiles
// were skipped due to missing package/module information.
func convertStrict(in io.Reader, out io.Writer, ignore *Ignore, pkgDir ...string) error {
	skipped, err := convertInternal(in, out, ignore, pkgDir...)
	if err != nil {
		return err
	}
	if skipped > 0 {
		return fmt.Errorf("%d profile(s) skipped due to missing package/module information", skipped)
	}
	return nil
}

func convertInternal(in io.Reader, out io.Writer, ignore *Ignore, pkgDir ...string) (int, error) {
	profiles, err := ParseProfiles(in, ignore)
	if err != nil {
		return 0, err
	}

	var dir string
	if len(pkgDir) > 0 {
		dir = pkgDir[0]
	}
	pkgs, err := getPackages(profiles, dir)
	if err != nil {
		return 0, err
	}

	sources := make([]*Source, 0)
	pkgMap := make(map[string]*packages.Package)
	for _, pkg := range pkgs {
		// packages.Load can return packages with nil Module when the
		// package is from the standard library, is built in GOPATH mode,
		// or when dependency resolution fails (e.g. private repo auth).
		if pkg.Module == nil {
			continue
		}
		sources = appendIfUnique(sources, pkg.Module.Dir)
		pkgMap[pkg.ID] = pkg
	}

	coverage := Coverage{Sources: sources, Packages: nil, Timestamp: time.Now().UnixNano() / int64(time.Millisecond)}
	skipped, err := coverage.parseProfiles(profiles, pkgMap, ignore)
	if err != nil {
		return skipped, err
	}

	_, _ = fmt.Fprint(out, xml.Header)
	_, _ = fmt.Fprintln(out, coberturaDTDDecl)

	encoder := xml.NewEncoder(out)
	encoder.Indent("", "  ")
	if err := encoder.Encode(coverage); err != nil {
		return skipped, err
	}

	_, _ = fmt.Fprintln(out)
	return skipped, nil
}

func getPackages(profiles []*Profile, dir string) ([]*packages.Package, error) {
	if len(profiles) == 0 {
		return []*packages.Package{}, nil
	}

	var pkgNames []string
	for _, profile := range profiles {
		pkgNames = append(pkgNames, getPackageName(profile.FileName))
	}
	return packages.Load(&packages.Config{Mode: packages.NeedFiles | packages.NeedModule, Dir: dir}, pkgNames...)
}

func appendIfUnique(sources []*Source, dir string) []*Source {
	for _, source := range sources {
		if source.Path == dir {
			return sources
		}
	}
	return append(sources, &Source{dir})
}

func getPackageName(filename string) string {
	pkgName, _ := filepath.Split(filename)
	// TODO(boumenot): Windows vs. Linux
	return strings.TrimRight(strings.TrimRight(pkgName, "\\"), "/")
}

func findAbsFilePath(pkg *packages.Package, profileName string) (string, error) {
	filename := filepath.Base(profileName)
	for _, fullpath := range pkg.GoFiles {
		if filepath.Base(fullpath) == filename {
			return fullpath, nil
		}
	}
	return "", fmt.Errorf("unable to determine file path for %s", profileName)
}

func (cov *Coverage) parseProfiles(profiles []*Profile, pkgMap map[string]*packages.Package, ignore *Ignore) (int, error) {
	cov.Packages = []*Package{}
	skipped := 0
	for _, profile := range profiles {
		pkgName := getPackageName(profile.FileName)
		pkgPkg := pkgMap[pkgName]
		skip, err := cov.parseProfile(profile, pkgPkg, ignore)
		if err != nil {
			return skipped, err
		}
		if skip {
			skipped++
		}
	}
	cov.LinesValid = cov.NumLines()
	cov.LinesCovered = cov.NumLinesWithHits()
	cov.LineRate = cov.HitRate()
	return skipped, nil
}

func (cov *Coverage) parseProfile(profile *Profile, pkgPkg *packages.Package, ignore *Ignore) (bool, error) {
	if pkgPkg == nil || pkgPkg.Module == nil {
		fmt.Fprintf(os.Stderr, "warning: skipping profile %s: no package/module information available\n", profile.FileName)
		return true, nil
	}
	fileName := profile.FileName[len(pkgPkg.Module.Path)+1:]
	absFilePath, err := findAbsFilePath(pkgPkg, profile.FileName)
	if err != nil {
		return false, err
	}

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, absFilePath, nil, 0)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(absFilePath)
	if err != nil {
		return false, err
	}

	if ignore.Match(fileName, data) {
		return false, nil
	}

	pkgPath, _ := filepath.Split(fileName)
	pkgPath = strings.TrimRight(strings.TrimRight(pkgPath, "/"), "\\")
	pkgPath = filepath.Join(pkgPkg.Module.Path, pkgPath)
	// TODO(boumenot): package paths are not file paths, there is a consistent separator
	pkgPath = strings.Replace(pkgPath, "\\", "/", -1)

	var pkg *Package
	for _, p := range cov.Packages {
		if p.Name == pkgPath {
			pkg = p
		}
	}
	if pkg == nil {
		pkg = &Package{Name: pkgPkg.ID, Classes: []*Class{}}
		cov.Packages = append(cov.Packages, pkg)
	}

	// Get the set of valid lines, but only if we're ignoring non-code lines. We set up the struct
	// regardless so that the array on fileVisitor isn't nil. It shouldnt matter, but just in case!
	validVisitor := &validVisitor{
		fset:       fset,
		validLines: make(map[int]bool),
	}
	if ignore.NonCodeLines {
		ast.Walk(validVisitor, parsed)
	}

	visitor := &fileVisitor{
		fset:               fset,
		fileName:           fileName,
		fileData:           data,
		classes:            make(map[string]*Class),
		pkg:                pkg,
		profile:            profile,
		validLines:         validVisitor.validLines,
		ignoreNonCodeLines: ignore.NonCodeLines,
	}
	ast.Walk(visitor, parsed)
	pkg.LineRate = pkg.HitRate()
	return false, nil
}

type validVisitor struct {
	fset       *token.FileSet
	validLines map[int]bool
}

func (v *validVisitor) Visit(node ast.Node) ast.Visitor {
	// Add the line for this node to the list of valid lines.
	if node != nil {
		v.validLines[v.fset.Position(node.Pos()).Line] = true
	}
	return v
}

type fileVisitor struct {
	fset               *token.FileSet
	fileName           string
	fileData           []byte
	pkg                *Package
	classes            map[string]*Class
	profile            *Profile
	validLines         map[int]bool
	ignoreNonCodeLines bool
}

func (v *fileVisitor) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.FuncDecl:
		class := v.classForFunc(n)
		method := v.methodFromNode(n, n.Name.Name)
		method.LineRate = method.Lines.HitRate()
		class.Methods = append(class.Methods, method)
		class.Lines = append(class.Lines, method.Lines...)
		class.LineRate = class.Lines.HitRate()
		// Return nil to stop descent — all lines in the function body
		// are already counted by methodFromNode using the Pos/End range.
		// This prevents local var closures from being double-counted.
		return nil
	case *ast.GenDecl:
		for _, spec := range n.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) == 0 || len(vs.Values) == 0 {
				continue
			}
			for i, val := range vs.Values {
				funcLit, isFuncLit := val.(*ast.FuncLit)
				if !isFuncLit {
					continue
				}
				name := vs.Names[i].Name
				class := v.classForFunc(nil)
				// Use the FuncLit node for Pos/End so the range covers
				// the function body, not the entire var declaration.
				method := v.methodFromNode(funcLit, name)
				method.LineRate = method.Lines.HitRate()
				class.Methods = append(class.Methods, method)
				class.Lines = append(class.Lines, method.Lines...)
				class.LineRate = class.Lines.HitRate()
			}
		}
		return nil
	}
	return v
}

// methodFromNode creates a Method by scanning coverage blocks that overlap
// with the AST node's source range.
func (v *fileVisitor) methodFromNode(n ast.Node, name string) *Method {
	method := &Method{Name: name}
	method.Lines = []*Line{}

	start := v.fset.Position(n.Pos())
	end := v.fset.Position(n.End())
	startLine := start.Line
	startCol := start.Column
	endLine := end.Line
	endCol := end.Column
	// The blocks are sorted, so we can stop counting as soon as we reach the end of the relevant block.
	for _, b := range v.profile.Blocks {
		if b.StartLine > endLine || (b.StartLine == endLine && b.StartCol >= endCol) {
			// Past the end of the function.
			break
		}
		if b.EndLine < startLine || (b.EndLine == startLine && b.EndCol <= startCol) {
			// Before the beginning of the function
			continue
		}
		for i := b.StartLine; i <= b.EndLine; i++ {
			if !v.ignoreNonCodeLines || v.validLines[i] {
				method.Lines.AddOrUpdateLine(i, int64(b.Count), v.profile.Mode)
			}
		}
	}
	return method
}

// classForFunc returns the Class for a function. For FuncDecl nodes, the
// class is determined by the receiver type (or "-" for package-level funcs).
// For variable functions (nil FuncDecl), the class is "-" (same as
// package-level functions without receivers). When -by-files is set, the
// file path is always used as the class name.
func (v *fileVisitor) classForFunc(n *ast.FuncDecl) *Class {
	var className string
	if byFiles {
		// NOTE(boumenot): ReportGenerator creates links that collide if names are not distinct.
		// The work around is to generate a fully qualified name based on the file path.
		className = strings.Replace(v.fileName, "/", ".", -1)
		className = strings.Replace(className, "\\", ".", -1)
	} else if n != nil {
		className = v.recvName(n)
	} else {
		className = "-"
	}
	class := v.classes[className]
	if class == nil {
		class = &Class{Name: className, Filename: v.fileName, Methods: []*Method{}, Lines: []*Line{}}
		v.classes[className] = class
		v.pkg.Classes = append(v.pkg.Classes, class)
	}
	return class
}

func (v *fileVisitor) recvName(n *ast.FuncDecl) string {
	if n.Recv == nil {
		return "-"
	}
	recv := n.Recv.List[0].Type
	start := v.fset.Position(recv.Pos())
	end := v.fset.Position(recv.End())
	name := string(v.fileData[start.Offset:end.Offset])
	return strings.TrimSpace(strings.TrimLeft(name, "*"))
}
