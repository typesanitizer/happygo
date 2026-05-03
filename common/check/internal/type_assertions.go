// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package internal

import (
	"github.com/typesanitizer/happygo/common/check"
	"github.com/typesanitizer/happygo/common/fsx"
)

var _ check.SnapshotFS = (fsx.FS)(nil)
