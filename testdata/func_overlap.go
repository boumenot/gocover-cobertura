// +build testdata
package testdata

// FuncOverlap has coverage blocks that overlap on line boundaries.
// When run with -coverpkg, go test may produce multiple blocks that
// share the same start line but have different end lines. The
// deduplication logic must correctly merge these.
func FuncOverlap(x int) int {
	if x > 0 {
		x = x * 2
	}
	return x
}
