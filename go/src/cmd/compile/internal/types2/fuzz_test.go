// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types2_test

import (
	"bytes"
	"cmd/compile/internal/syntax"
	types2 "cmd/compile/internal/types2"
	"internal/parser_testcases"
	"regexp"
	"testing"
)

var shortErrorComment = regexp.MustCompile(`/\* ERROR(?: HERE)? "[^"]*" \*/`)

var typeCheckSeeds = [...]string{
	"package p\n",
	"package p\nvar _ = 0\n",
	"package p\ntype T int\nvar _ T\n",
	"package p\nfunc f[T any](x T) T { return x }\n",
	"package p\ntype T interface{ ~int | ~string }\n",
	"package p\nimport \"fmt\"\nvar _ fmt.Stringer\n",
	`package p

type Ring[A, B, C any] struct {
	L *Ring[B, C, A]
	R *Ring[C, A, B]
}
`,
	`package p

type Constraint[T any] interface {
	~[]T | ~chan T
}

func Use[T any, P Constraint[T]](p P) {
	_ = p
}
`,
	`package p

type Box[T any] struct {
	value T
}

func (b Box[T]) Value() T { return b.value }

var _ = Box[int]{}.Value
`,
	`package p

type Pair[A, B any] = struct {
	X A
	Y B
}

var _ Pair[int, string]
`,
	`package p

type Slice[T any] = []T
type NamedSlice[T any] Slice[T]

var _ NamedSlice[int]
`,
	`package p

type Tree[T any] struct {
	*Node[T]
	Value T
}

type Node[T any] struct {
	*Tree[T]
}

type Inst = *Tree[int]

var _ Inst
`,
	`package p

type T[P any] *T[P]

var _ T[int]
`,
	`package p

type A = B
type B = C
type C = int

var _ A
`,
	`package a

var x T[B]

type T[_ any] struct{}
type A T[B]
type B = T[A]
`,
	`package p

type T[P any] struct {
	next *T[P]
}

type Inst = T[int]

var _ Inst
`,
	`package p

func F[T any](x T) { F(&x) }
`,
	`package p

import "unsafe"

var _ unsafe.Pointer

type A[T any] struct {
	_ A[*T]
}
`,
	`package p

type A = *A
`,
	`package p

type A = B
type B = *A
`,
	`package p

type A[T any] = B[T]
type B[T any] = *A[T]
`,
	`package p

type T[P any] struct{}

func (T[P]) m() {}

var _ = T[int].m
`,
	`package p

type A[_ any] = any

func _[_ A[int, string]]() {}

type T[_ any] any

func _[_ T[int, string]]() {}
`,
	`package p

type F[T any] func(func(F[T]))

func f(F[int])      {}
func g[T any](F[T]) {}

func _() {
	g(f)
}

type List[T any] func(T, func(T, List[T]) T) T

func nil[T any](n T, _ List[T]) T        { return n }
func cons[T any](h T, t List[T]) List[T] { return func(n T, f func(T, List[T]) T) T { return f(h, t) } }

func nums[T any](t T) List[T] {
	return cons(t, cons(t, nil[T]))
}
`,
	`package p

type S[T any] struct{}

var V = S[any]{}

func (fs *S[T]) M(V.M) {}

type S1[T any] V1.M
type V1 = S1[any]

type S2[T any] struct{}
type V2 = S2[any]

func (fs *S2[T]) M(x V2.M) {}
`,
	`package p

type Interface[T any] interface {
	m(Interface[T])
}

func f[S []Interface[T], T any](S) {}

func _() {
	var s []Interface[int]
	f(s)
}
`,
	`package p

func f[P any](P) P { panic(0) }

var v func(string) int = f

func _() func(string) int {
	return f
}
`,
	`package p

func _() {
	NewS().M()
}

type S struct{}

func NewS[T any]() *S { panic(0) }

func (_ *S[T]) M()
`,
}

func FuzzTypeCheck(f *testing.F) {
	for _, seed := range parser_testcases.Valids() {
		f.Add([]byte(seed))
	}
	for _, seed := range parser_testcases.Invalids() {
		f.Add([]byte(shortErrorComment.ReplaceAllString(seed, "")))
	}
	for _, seed := range typeCheckSeeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(_ *testing.T, src []byte) {
		hadParseError := false
		file, _ := syntax.Parse(syntax.NewFileBase("fuzz.go"), bytes.NewReader(src), func(error) {
			hadParseError = true
		}, nil, 0)
		if hadParseError || file == nil || file.PkgName == nil {
			return
		}

		conf := types2.Config{
			Importer: defaultImporter(),
			Error:    func(error) {},
		}
		info := &types2.Info{
			Types:        make(map[syntax.Expr]types2.TypeAndValue),
			Instances:    make(map[*syntax.Name]types2.Instance),
			Defs:         make(map[*syntax.Name]types2.Object),
			Uses:         make(map[*syntax.Name]types2.Object),
			Implicits:    make(map[syntax.Node]types2.Object),
			Selections:   make(map[*syntax.SelectorExpr]*types2.Selection),
			Scopes:       make(map[syntax.Node]*types2.Scope),
			FileVersions: make(map[*syntax.PosBase]string),
		}

		_, _ = conf.Check("fuzz/p", []*syntax.File{file}, info)
	})
}
