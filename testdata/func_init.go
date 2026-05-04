// +build testdata
package testdata

var initCalled bool

func init() {
	initCalled = true
}
