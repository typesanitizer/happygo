// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package time

import (
	"strconv"
	"strings"
	stdlib_time "time"

	"github.com/typesanitizer/happygo/common/assert"
)

// Duration is an elapsed time duration.
type Duration = stdlib_time.Duration

const (
	Nanosecond  Duration = stdlib_time.Nanosecond
	Microsecond Duration = stdlib_time.Microsecond
	Millisecond Duration = stdlib_time.Millisecond
	Second      Duration = stdlib_time.Second
	Minute      Duration = stdlib_time.Minute
	Hour        Duration = stdlib_time.Hour
)

// SystemClock provides access to the current system time.
type SystemClock interface {
	Now() SystemTime
}

// SystemTime is a wall-clock timestamp.
type SystemTime struct {
	value stdlib_time.Time
}

// NewSystemTime wraps a stdlib time value.
//
// This is intended for capability boundary packages such as syscaps. Most code
// should receive SystemTime values from a SystemClock instead.
func NewSystemTime(value stdlib_time.Time) SystemTime {
	return SystemTime{value: value}
}

// Unix returns the local SystemTime corresponding to the given Unix time.
func Unix(sec int64, nsec int64) SystemTime {
	return NewSystemTime(stdlib_time.Unix(sec, nsec))
}

func (t SystemTime) UTC() SystemTime {
	return NewSystemTime(t.value.UTC())
}

// Format returns t formatted according to pat.
func (t SystemTime) Format(pat Pattern) string {
	var b strings.Builder
	for _, spec := range pat.specs {
		switch spec.kind {
		case formatSpecKind_Fixed:
			b.WriteString(spec.text)
		case formatSpecKind_Year:
			writePaddedInt(&b, t.value.Year(), 4)
		case formatSpecKind_Month:
			writePaddedInt(&b, int(t.value.Month()), 2)
		case formatSpecKind_Day:
			writePaddedInt(&b, t.value.Day(), 2)
		default:
			return assert.PanicUnknownCase[string](spec.kind)
		}
	}
	return b.String()
}

// Pattern describes the textual representation of a time-like type.
//
// It can be used for formatting.
type Pattern struct {
	specs []spec
}

// NewPattern returns an empty formatting pattern.
func NewPattern() Pattern {
	return Pattern{specs: nil}
}

// Fixed appends literal text to p.
//
// The text is never interpreted as a stdlib time layout fragment; for example,
// Fixed("2006") formats as the literal string "2006".
func (p Pattern) Fixed(text string) Pattern {
	return p.append(spec{kind: formatSpecKind_Fixed, text: text})
}

// Year appends the astronomical year number with at least four digits.
//
// Year 0 is 1 BCE and formats as "0000"; the leading minus sign starts with
// year -1, which is 2 BCE.
func (p Pattern) Year() Pattern {
	return p.append(spec{kind: formatSpecKind_Year, text: ""})
}

// Month appends the month of the year as two digits, from "01" through "12".
func (p Pattern) Month() Pattern {
	return p.append(spec{kind: formatSpecKind_Month, text: ""})
}

// Day appends the day of the month as two digits, from "01" through "31".
func (p Pattern) Day() Pattern {
	return p.append(spec{kind: formatSpecKind_Day, text: ""})
}

type spec struct {
	kind formatSpecKind
	// text is set only when kind is formatSpecKind_Fixed.
	text string
}

type formatSpecKind uint8

const (
	formatSpecKind_Fixed formatSpecKind = iota + 1
	formatSpecKind_Year
	formatSpecKind_Month
	formatSpecKind_Day
)

func (p Pattern) append(spec spec) Pattern {
	p.specs = append(p.specs, spec)
	return p
}

func writePaddedInt(b *strings.Builder, value int, width int) {
	if value < 0 {
		b.WriteByte('-')
		value = -value
	}
	digits := strconv.Itoa(value)
	for i := len(digits); i < width; i++ {
		b.WriteByte('0')
	}
	b.WriteString(digits)
}
