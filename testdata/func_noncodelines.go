// +build testdata
package testdata

// FuncWithNonCodeLines has comments in to catch when they are/are not covered.
func FuncWithNonCodeLines(arg1 int) int {
	// Let's add some comments here. We're going to use powers of two so we know which are/are not covered.
	if arg1 == 0 {
		// Some more comments here.
		// We want two lines.
		arg1 += 1
	}

	// Some more comments here.
	// We want 4 lines.
	// 3.
	// 4.
	arg1 += 2

	return arg1
}
