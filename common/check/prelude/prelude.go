// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

// Package prelude provides terse test helpers intended for dot-import in tests.
package prelude

import "github.com/typesanitizer/happygo/common/check"

// Do turns a (T, error) pair into a harness-applied extractor.
func Do[T any](value T, err error) func(check.BasicHarness) T {
	return func(h check.BasicHarness) T {
		h.NoErrorf(err, "unexpected error")
		return value
	}
}

// DoMsg turns a (T, error) pair into a harness-applied extractor with a custom error message.
func DoMsg[T any](value T, err error) func(check.BasicHarness, string, ...any) T {
	return func(h check.BasicHarness, msg string, args ...any) T {
		h.NoErrorf(err, msg, args...)
		return value
	}
}
