// +build testdata
package testdata

// Type with a generic-looking name to test receiver extraction.
type MyService struct{}

// Value receiver.
func (s MyService) Handle(arg1 *int) {
	*arg1 = 1
}

// Pointer receiver.
func (s *MyService) Process(arg1 *int) {
	*arg1 = 2
}

// Embedded struct to test nested type receiver.
type Wrapper struct {
	MyService
}

func (w *Wrapper) Run(arg1 *int) {
	*arg1 = 3
}
