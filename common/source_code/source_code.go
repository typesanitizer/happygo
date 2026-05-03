// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

// Package source_code provides source-backed positions and snippets.
package source_code

import (
	"fmt"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/core/pathx"
)

// Position identifies a 1-based line and column within a file.
type Position struct {
	line   int32
	column int32
}

func NewPosition(line, column int32) Position {
	assert.Preconditionf(line > 0, "line must be >= 1, got %d", line)
	assert.Preconditionf(column > 0, "column must be >= 1, got %d", column)
	return Position{line: line, column: column}
}

// Line returns a 1-based line number.
func (p Position) Line() int32 {
	return p.line
}

// Column returns a 1-based column number.
//
// The exact semantics of "width" (particularly, how tabs and wide characters
// are interpreted) are left up to the producer of Positions.
func (p Position) Column() int32 {
	return p.column
}

func (p Position) String() string {
	return fmt.Sprintf("%d:%d", p.line, p.column)
}

// FilePosition identifies a position within a file rooted at an fsx.FS root.
type FilePosition struct {
	Path     pathx.RelPath
	Position Position
}

// Line returns a 1-based line number.
func (p FilePosition) Line() int32 {
	return p.Position.Line()
}

// Column returns a 1-based column number.
//
// The exact semantics of "width" (particularly, how tabs and wide characters
// are interpreted) are left up to the producer of the corresponding Position.
func (p FilePosition) Column() int32 {
	return p.Position.Column()
}

func (p FilePosition) String() string {
	return fmt.Sprintf("%s:%d:%d", p.Path, p.Line(), p.Column())
}

// Snippet identifies a piece of source text and where it came from.
type Snippet struct {
	Path     pathx.RelPath
	Position Position
	Text     string
}

func (s Snippet) FilePosition() FilePosition {
	return FilePosition{s.Path, s.Position}
}

// Line returns a 1-based line number.
func (s Snippet) Line() int32 {
	return s.Position.Line()
}

// Column returns a 1-based column number.
//
// The exact semantics of "width" (particularly, how tabs and wide characters
// are interpreted) are left up to the producer of the Snippet.
func (s Snippet) Column() int32 {
	return s.Position.Column()
}
