// +build testdata
package testdata

// GlobalVarFunc is a package-level variable function.
var GlobalVarFunc = func(arg1 *int) {
	if *arg1 != 0 {
		*arg1 = 1
	}
}

// NonFuncVar is a package-level variable that is NOT a function.
// The visitor must NOT create a method for this.
var NonFuncVar = "hello"

// FuncWithLocalVarFunc contains a local var function. The visitor must
// not double-count lines inside the closure.
func FuncWithLocalVarFunc(arg1 *int) {
	var localFn = func() {
		*arg1 = 2
	}
	localFn()
}

// FuncWithShortAssignClosure contains a closure assigned with :=.
// This is an AssignStmt, not a ValueSpec, so the visitor should NOT
// pick it up separately — it's already covered by the FuncDecl.
func FuncWithShortAssignClosure(arg1 *int) {
	localFn := func() {
		*arg1 = 3
	}
	localFn()
}
