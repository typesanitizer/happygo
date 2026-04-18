// Package fsx_testkit provides test helpers for fsx.FS values.
package fsx_testkit

import (
	"runtime"

	. "github.com/typesanitizer/happygo/common/core"
)

func FakeRoot() AbsPath {
	if runtime.GOOS == "windows" {
		return NewAbsPath(`C:\virtual-root`)
	}
	return NewAbsPath("/virtual-root")
}
