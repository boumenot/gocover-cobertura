// +build testdata
package testdata

import "sort"

// FuncWithClosureArg passes an anonymous function as an argument.
// The closure's lines should be covered by the enclosing FuncDecl.
func FuncWithClosureArg(data []int) {
	sort.Slice(data, func(i, j int) bool {
		return data[i] < data[j]
	})
}
