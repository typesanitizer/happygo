//go:build !windows

package fsx_testkit

import (
	"os"

	"github.com/typesanitizer/happygo/common/core/op"
)

// TrySymlink creates a symlink named link pointing at target if the platform
// supports it.
//
// If the operation is supported, returns [op.Supported] and the error from
// the operation.
//
// If the operation is not supported, returns [op.Unsupported] and nil.
func TrySymlink(target, link string) (op.PlatformSupport, error) {
	return op.Supported, os.Symlink(target, link)
}
