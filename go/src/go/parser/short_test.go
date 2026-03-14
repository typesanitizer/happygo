// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser

import (
	"internal/parser_testcases"
	"testing"
)

func TestValid(t *testing.T) {
	for _, src := range parser_testcases.Valids() {
		checkErrors(t, src, src, DeclarationErrors|AllErrors, false)
	}
}

// TestSingle is useful to track down a problem with a single short test program.
func TestSingle(t *testing.T) {
	const src = `package p; var _ = T{}`
	checkErrors(t, src, src, DeclarationErrors|AllErrors, true)
}

func TestInvalid(t *testing.T) {
	for _, src := range parser_testcases.Invalids() {
		checkErrors(t, src, src, DeclarationErrors|AllErrors, true)
	}
}
