// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

// Package pathx provides filepath utilities.
package pathx

import (
	"path/filepath"
	"strings"
)

// LexicallyContains reports whether child is lexically contained within root.
// Both paths are cleaned before comparison.
func LexicallyContains(root, child string) bool {
	rel, err := filepath.Rel(root, filepath.Join(root, child))
	return err == nil && !strings.HasPrefix(rel, "..")
}
