package fsx_walk_test

import (
	"fmt"
	"iter"
	"sort"

	"github.com/typesanitizer/happygo/common/collections"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/core/result"
	"github.com/typesanitizer/happygo/common/fsx"
	"github.com/typesanitizer/happygo/common/fsx/fsx_testkit"
	"github.com/typesanitizer/happygo/common/fsx/fsx_walk"
)

// Example_iterative shows how to drive a [fsx_walk.WalkNonDet] result
// iteratively, using a [collections.Stack] of pending child iterators
// instead of recursion. This pattern is useful when recursion depth
// is unbounded or when descent decisions depend on caller state.
func Example_iterative() {
	root := fsx_testkit.FakeRoot().JoinComponents("example-root")
	fs, err := fsx.MemMap(root)
	if err != nil {
		panic(err)
	}
	mustWrite := func(path, content string) {
		rel := pathx.NewRelPath(path)
		if dir, ok := rel.Dir().Get(); ok {
			if err := fs.MkdirAll(dir, 0o755); err != nil {
				panic(err)
			}
		}
		if err := fs.WriteFile(rel, []byte(content), 0o644); err != nil {
			panic(err)
		}
	}
	mustWrite("a/b.txt", "b")
	mustWrite("a/sub/c.txt", "c")
	mustWrite("d.txt", "d")

	entries, err := fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: false})
	if err != nil {
		panic(err)
	}

	type frame struct {
		prefix string
		next   func() (result.Result[fsx_walk.FSWalkEntry], bool)
		stop   func()
	}
	push := func(stack *collections.Stack[frame], prefix string, seq iter.Seq[result.Result[fsx_walk.FSWalkEntry]]) {
		next, stop := iter.Pull(seq)
		stack.Push(frame{prefix: prefix, next: next, stop: stop})
	}

	stack := collections.NewStack[frame]()
	push(&stack, "", entries)

	var paths []string
	for !stack.IsEmpty() {
		top := stack.Pop()
		entryRes, ok := top.next()
		if !ok {
			top.stop()
			continue
		}
		// Re-push: the current frame still has more siblings to yield.
		stack.Push(top)
		entry, err := entryRes.Get()
		if err != nil {
			panic(err)
		}
		path := top.prefix + entry.Name().String()
		paths = append(paths, path)
		if entry.IsDir() {
			push(&stack, path+"/", entry.ChildrenNonDet())
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Println(p)
	}
	// Output:
	// a
	// a/b.txt
	// a/sub
	// a/sub/c.txt
	// d.txt
}
