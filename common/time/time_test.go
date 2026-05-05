// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package time

import (
	"testing"
	stdlib_time "time"

	"github.com/typesanitizer/happygo/common/check"
)

func TestPatternFormatsDate(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	timestamp := NewSystemTime(stdlib_time.Date(2026, stdlib_time.May, 5, 0, 0, 0, 0, stdlib_time.UTC))
	pattern := NewPattern().Year().Fixed("-").Month().Fixed("-").Day()

	check.AssertSame(h, "2026-05-05", timestamp.Format(pattern), "formatted date")
}

func TestPatternFixedTextIsLiteral(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	timestamp := Unix(0, 0).UTC()
	pattern := NewPattern().Fixed("2006-01-02")

	check.AssertSame(h, "2006-01-02", timestamp.Format(pattern), "fixed text")
}

func TestPatternFormatsNegativeYear(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	// stdlib time.Time supports astronomical year numbering: year 0 is 1 BCE,
	// year -1 is 2 BCE, and so on. Preserve the sign when formatting these years.
	timestamp := NewSystemTime(stdlib_time.Date(-12, stdlib_time.May, 5, 0, 0, 0, 0, stdlib_time.UTC))
	pattern := NewPattern().Year().Fixed("-").Month().Fixed("-").Day()

	check.AssertSame(h, "-0012-05-05", timestamp.Format(pattern), "formatted date")
}
