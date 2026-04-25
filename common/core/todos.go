package core

import "fmt"

// Unimplementedf is a marker function used to mark unimplemented code paths.
//
// NOTE: This does not create an assert.AssertionError, because Unimplementedf
// is meant to be used only for local development, and should not be used
// in code which is merged.
func Unimplementedf(msg string, args ...any) {
	panic(fmt.Sprintf("unimplemented: "+msg, args))
}
