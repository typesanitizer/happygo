// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syntax

import (
	"bytes"
	"internal/parser_testcases"
	"regexp"
	"testing"
)

var shortErrorComment = regexp.MustCompile(`/\* ERROR(?: HERE)? "[^"]*" \*/`)

func FuzzParse(f *testing.F) {
	for _, seed := range parser_testcases.Valids() {
		f.Add([]byte(seed))
	}
	for _, seed := range parser_testcases.Invalids() {
		f.Add([]byte(shortErrorComment.ReplaceAllString(seed, "")))
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		base := NewFileBase("fuzz.go")
		file, err := Parse(base, bytes.NewReader(src), nil, nil, 0)
		if file == nil {
			if err == nil {
				t.Fatalf("Parse returned nil file without error")
			}
			return
		}

		_, _ = Parse(NewFileBase("fuzz.go"), bytes.NewReader(src), func(error) {}, nil, 0)

		if err != nil {
			return
		}

		var printed bytes.Buffer
		if _, err := Fprint(&printed, file, LineForm); err != nil {
			t.Fatalf("Fprint failed: %v", err)
		}

		reparsed, err := Parse(NewFileBase("printed.go"), bytes.NewReader(printed.Bytes()), nil, nil, 0)
		if err != nil {
			t.Fatalf("reparse of printed AST failed: %v", err)
		}

		var printedAgain bytes.Buffer
		if _, err := Fprint(&printedAgain, reparsed, LineForm); err != nil {
			t.Fatalf("second Fprint failed: %v", err)
		}

		if !bytes.Equal(printed.Bytes(), printedAgain.Bytes()) {
			t.Fatalf("printed AST was not idempotent")
		}
	})
}
