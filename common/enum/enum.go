// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package enum

import "github.com/typesanitizer/happygo/common/assert"

// --- Aliases for external use ---

// Filter represents a filter which can be one of two cases:
//
// - FilterKind_All: Allows matching against any enum case.
// - FilterKind_Exact: Only exactly matches one enum case.
type Filter[E AllIterable[E]] = EnumFilter[E]

// FilterKind represents the case of a Filter.
type FilterKind = EnumFilterKind

// --- Package implementation ---

// AllIterable represents enums which expose an All()
// method which allows someone to iterate over the cases.
type AllIterable[E any] interface {
	// All should really be a static method, but Go interfaces
	// don't support those.
	All() []E
}

// See doc comment for Filter.
type EnumFilter[E AllIterable[E]] struct {
	specificData E
	kind         EnumFilterKind
}

// NewAllFilter constructs a new filter which matches
// any case of the enum E.
func NewAllFilter[E AllIterable[E]]() EnumFilter[E] {
	var zero E
	return EnumFilter[E]{specificData: zero, kind: FilterKind_All}
}

// NewExactFilter constructs a new filter which exactly
// matches 'value'.
func NewExactFilter[E AllIterable[E]](value E) EnumFilter[E] {
	return EnumFilter[E]{specificData: value, kind: FilterKind_Exact}
}

// Kind represents what kind of filter this is.
func (ef EnumFilter[E]) Kind() EnumFilterKind {
	return ef.kind
}

// ExactValue gets the value to be matched for a filter with
// kind FilterKind_Exact.
func (ef EnumFilter[E]) ExactValue() E {
	assert.Preconditionf(ef.kind == FilterKind_Exact, "calling Exact for case %v", ef.kind)
	return ef.specificData
}

// See doc comment for FilterKind.
type EnumFilterKind uint8

const (
	FilterKind_All EnumFilterKind = iota + 1
	FilterKind_Exact
)

func (ef EnumFilterKind) String() string {
	switch ef {
	case FilterKind_All:
		return "all"
	case FilterKind_Exact:
		return "exact"
	default:
		return assert.PanicUnknownCase[string](ef)
	}
}
