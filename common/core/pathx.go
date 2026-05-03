// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package core

import "github.com/typesanitizer/happygo/common/core/pathx"

// Path types for manipulating host platform paths.
// See the pathx package for details.

type AbsPath = pathx.AbsPath

type RelPath = pathx.RelPath

type RootRelPath = pathx.RootRelPath

func NewAbsPath(path string) AbsPath {
	return pathx.NewAbsPath(path)
}

func NewRelPath(path string) RelPath {
	return pathx.NewRelPath(path)
}

func NewRootRelPath(root AbsPath, subpath RelPath) RootRelPath {
	return pathx.NewRootRelPath(root, subpath)
}
