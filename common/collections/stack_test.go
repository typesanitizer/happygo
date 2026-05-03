// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package collections_test

import (
	"slices"
	"testing"
	
	"pgregory.net/rapid"

	"github.com/typesanitizer/happygo/common/check"
	"github.com/typesanitizer/happygo/common/collections"
)

func TestStack(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		h := check.NewBasic(t)

		stack := collections.NewStack[int]()
		pushes := rapid.SliceOfN(rapid.Int(), 0, 10).Draw(t, "pushes")
		for _, p := range pushes {
			stack.Push(p)
		}

		wantLen := len(pushes)
		popped := []int{}
		for !stack.IsEmpty() {
			check.AssertSame(h, wantLen, stack.Len(), "len")
			popped = append(popped, stack.Pop())
			wantLen--
		}
		check.AssertSame(h, 0, stack.Len(), "len")

		reversedPushes := slices.Clone(pushes)
		slices.Reverse(reversedPushes)

		check.AssertSame(h, reversedPushes, popped, "pop order")
	})
}
