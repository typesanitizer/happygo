// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

// Package pathx_testkit provides rapid generators for pathx types.
package pathx_testkit

import (
	"path/filepath"
	"strings"

	"pgregory.net/rapid"

	"github.com/typesanitizer/happygo/common/core/pathx"
)

// ComponentGen generates a short, safe-looking path component.
// It emits no separators or dot-only names, and on Unix it will not produce
// invalid path component names. On Windows it may still produce reserved
// device names (for example CON or COM1) or names ending in '.', so it should
// not be used for real filesystem operations there.
func ComponentGen() *rapid.Generator[string] {
	return rapid.StringMatching(`[A-Za-z0-9][A-Za-z0-9._-]{0,7}`)
}

// SafeRelPathGen generates a relative path that is guaranteed not to escape
// its parent via ".." components. It may include "." and ".." components,
// but the net depth never goes negative.
func SafeRelPathGen() *rapid.Generator[pathx.RelPath] {
	return rapid.Custom(func(t *rapid.T) pathx.RelPath {
		componentCount := rapid.IntRange(0, 12).Draw(t, "component_count")
		components := make([]string, 0, componentCount)
		depth := 0
		for range componentCount {
			kind := rapid.IntRange(0, 3).Draw(t, "kind")
			switch kind {
			case 0:
				components = append(components, ".")
			case 1:
				if depth > 0 && rapid.Bool().Draw(t, "go_up") {
					components = append(components, "..")
					depth--
					continue
				}
				fallthrough
			default:
				components = append(components, ComponentGen().Draw(t, "token"))
				depth++
			}
		}
		if len(components) == 0 {
			return pathx.Dot()
		}
		// Use strings.Join to avoid the Clean operation invoked by filepath.Join.
		return pathx.NewRelPath(strings.Join(components, string(filepath.Separator)))
	})
}

// EscapingRelPathGen generates a relative path that escapes its parent via
// ".." components. The ".." and tail components are interleaved randomly,
// but the net depth is always negative at the end.
func EscapingRelPathGen() *rapid.Generator[pathx.RelPath] {
	return rapid.Custom(func(t *rapid.T) pathx.RelPath {
		upCount := rapid.IntRange(1, 6).Draw(t, "up_count")
		tailCount := rapid.IntRange(0, upCount-1).Draw(t, "tail_count")
		upsLeft := upCount
		tailsLeft := tailCount
		components := make([]string, 0, upCount+tailCount)
		for upsLeft > 0 || tailsLeft > 0 {
			switch {
			case upsLeft > 0 && tailsLeft > 0:
				if rapid.Bool().Draw(t, "up_or_tail") {
					components = append(components, "..")
					upsLeft--
				} else {
					components = append(components, ComponentGen().Draw(t, "tail"))
					tailsLeft--
				}
			case upsLeft > 0:
				components = append(components, "..")
				upsLeft--
			default:
				components = append(components, ComponentGen().Draw(t, "tail"))
				tailsLeft--
			}
		}
		// Use strings.Join to avoid the Clean operation invoked by filepath.Join.
		return pathx.NewRelPath(strings.Join(components, string(filepath.Separator)))
	})
}
