package syscaps_test

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/check"
	. "github.com/typesanitizer/happygo/common/check/prelude"
	"github.com/typesanitizer/happygo/common/collections"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/internal/constants"
	"github.com/typesanitizer/happygo/common/syscaps"
)

func TestFSReadDirBatched(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	repoFS := Do(syscaps.FS(NewAbsPath(t.TempDir())))(h)

	rapid.Check(h.T(), func(t *rapid.T) {
		h := check.NewBasic(t)
		entryCount := rapid.IntRange(0, constants.ReadDirBatchSize*3).Draw(t, "entry_count")
		parentDir := Do(repoFS.MkdirTemp(pathx.Dot(), "entries-"))(h)

		want := collections.NewSet[string]()
		for i := range entryCount {
			name := fmt.Sprintf("file-%03d.txt", i)
			fileRel := parentDir.JoinComponents(name)
			h.NoErrorf(repoFS.WriteFile(fileRel, []byte("data"), 0o644), "WriteFile(%q)", fileRel)
			want.InsertNew(name)
		}

		got := collections.NewSet[string]()
		for entryRes := range repoFS.ReadDir(parentDir) {
			entry := Do(entryRes.Get())(h)
			name := entry.BaseName().String()
			got.InsertNew(name)

			info := Do(entry.Info())(h)
			h.Assertf(info.Name() == name, "Info(%q).Name() = %q, want %q", name, info.Name(), name)
			h.Assertf(!entry.IsDir(), "ReadDir(%q) returned directory entry %q, want file", parentDir, name)
			h.Assertf(!info.IsDir(), "Info(%q).IsDir() = true, want false", name)
		}

		check.AssertSame(h, collections.SortedValues(want), collections.SortedValues(got), "ReadDir entries")
	})
}

func TestFSReadDirOnFileReturnsError(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	repoFS := Do(syscaps.FS(NewAbsPath(t.TempDir())))(h)

	fileRel := NewRelPath("file.txt")
	h.NoErrorf(repoFS.WriteFile(fileRel, []byte("data"), 0o644), "WriteFile(%q)", fileRel)

	gotAny := false
	for entryRes := range repoFS.ReadDir(fileRel) {
		gotAny = true
		_, err := entryRes.Get()
		h.Assertf(err != nil, "ReadDir(%q) unexpectedly succeeded", fileRel)
	}
	h.Assertf(gotAny, "ReadDir(%q) produced no result", fileRel)
}

func TestFSMkdirTempRejectsEmptyPattern(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	repoFS := Do(syscaps.FS(NewAbsPath(t.TempDir())))(h)
	want := assert.AssertionError{Fmt: "precondition violation: pattern is empty", Args: nil}
	h.AssertPanicsWith(want, func() {
		_, _ = repoFS.MkdirTemp(pathx.Dot(), "")
	})
}
