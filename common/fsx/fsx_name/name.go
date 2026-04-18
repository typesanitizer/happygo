package fsx_name

import (
	"fmt"
	"strings"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/core/pathx"
)

// --- Aliases for external use ---

// ParseError is one of two cases:
//
//  1. ParseErrorKind_EmptyName.
//  2. ParseErrorKind_HasPathSeparators: The original name can
//     be retrieved using the [ParseError.Name] method.
type ParseError = NameParseError

// ParseErrorKind tracks the case of a ParseError.
type ParseErrorKind = NameParseErrorKind

// --- Package implementation ---

// Name is a validated single-component file or directory name.
// It is guaranteed to be non-empty and contain no path separators.
type Name struct {
	value string
}

func (n Name) String() string {
	return n.value
}

func (n Name) Compare(p Name) int {
	return strings.Compare(n.value, p.value)
}

// New creates a Name from s.
//
// Pre-conditions:
// 1. s is non-empty.
// 2. s does not contain any path separators.
func New(s string) Name {
	n, err := Parse(s)
	assert.Preconditionf(err == nil, "%v", err)
	return n
}

// Parse attempts to parse the argument as a valid name
// for the host operating system.
func Parse(s string) (Name, error) {
	if s == "" {
		return Name{}, &NameParseError{ParseErrorKind_EmptyName, ""}
	}
	if pathx.HasPathSeparators(s) {
		return Name{}, &NameParseError{ParseErrorKind_HasPathSeparators, s}
	}
	return Name{s}, nil
}

// See ParseError for doc comment.
type NameParseError struct {
	kind NameParseErrorKind
	name string
}

func (e *NameParseError) Kind() NameParseErrorKind {
	return e.kind
}

// Pre-condition: Kind() should be ParseErrorKind_HasPathSeparators
func (e *NameParseError) Name() string {
	assert.Preconditionf(e.kind == ParseErrorKind_HasPathSeparators, "e.kind should be ParseErrorKind_HasPathSeparators")
	return e.name
}

func (e *NameParseError) Error() string {
	switch e.kind {
	case ParseErrorKind_EmptyName:
		return "empty file/directory name"
	case ParseErrorKind_HasPathSeparators:
		return fmt.Sprintf("name %q contains path separator(s)", e.name)
	default:
		return assert.PanicUnknownCase[string](e.kind)
	}
}

type NameParseErrorKind uint8

const (
	ParseErrorKind_EmptyName NameParseErrorKind = iota + 1
	ParseErrorKind_HasPathSeparators
)
