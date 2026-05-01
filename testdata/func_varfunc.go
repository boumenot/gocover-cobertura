// +build testdata
package testdata

// GlobalVarFunc is a package-level variable function. Coverage for these
// functions is recorded by `go test` but was previously dropped by
// gocover-cobertura because the AST visitor only matched *ast.FuncDecl.
var GlobalVarFunc = func(arg1 *int) {
	if *arg1 != 0 {
		*arg1 = 1
	}
}

// FuncWithLocalVarFunc is a regular function that contains a local
// variable function declared with `var` syntax. If the visitor naively
// matches *ast.ValueSpec, lines inside the closure will be double-counted:
// once as part of FuncWithLocalVarFunc and again as a separate "method".
func FuncWithLocalVarFunc(arg1 *int) {
	var localFn = func() {
		*arg1 = 2
	}
	localFn()
}
