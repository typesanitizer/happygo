package internal

import (
	"github.com/typesanitizer/happygo/common/check"
	"github.com/typesanitizer/happygo/common/fsx"
)

var _ check.SnapshotFS = (fsx.FS)(nil)
