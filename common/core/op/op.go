// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

// Package op provides small operation-result newtypes for APIs where a named
// boolean result is clearer than a raw bool.
//
// NOTE: Avoid adding newtypes if an operation can return data appropriately.
// For example, a remove operation on a map-like type should return an Option of
// the old value.
package op

// InsertResult reports whether an insert added a new value or kept an existing one.
type InsertResult bool

const (
	InsertedNew InsertResult = true
	KeptOld     InsertResult = false
)

func (res InsertResult) AsBool() bool {
	return bool(res)
}

// PlatformSupport reports whether an operation is supported on the current
// platform.
type PlatformSupport bool

const (
	Supported   PlatformSupport = true
	Unsupported PlatformSupport = false
)

func (s PlatformSupport) IsSupported() bool {
	return bool(s)
}

func (s PlatformSupport) IsUnsupported() bool {
	return !bool(s)
}
