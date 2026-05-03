// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package fsx

import "github.com/typesanitizer/happygo/common/fsx/fsx_name"

type Name = fsx_name.Name

// NewName creates a Name from name.
//
// Pre-conditions:
// 1. name is non-empty.
// 2. name does not contain any path separators.
func NewName(name string) Name {
	return fsx_name.New(name)
}
