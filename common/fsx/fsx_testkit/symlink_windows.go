//go:build windows

package fsx_testkit

import (
	"os"
	"syscall"

	"github.com/typesanitizer/happygo/common/core/op"
	"github.com/typesanitizer/happygo/common/errorx"
)

// TrySymlink creates a symlink named link pointing at target if the platform
// supports it.
//
// If the operation is supported, returns [op.Supported] and the error from
// the operation.
//
// If the operation is not supported, returns [op.Unsupported] and nil.
func TrySymlink(target, link string) (op.PlatformSupport, error) {
	err := os.Symlink(target, link)
	switch {
	case err == nil:
		return op.Supported, nil
	case errorx.GetRootCauseAsValue(err, syscall.EWINDOWS),
		errorx.GetRootCauseAsValue(err, syscall.ERROR_PRIVILEGE_NOT_HELD):
		return op.Unsupported, nil
	default:
		return op.Supported, err
	}
}
