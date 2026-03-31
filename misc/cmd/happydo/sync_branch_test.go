package main

import (
	"strings"
	"testing"

	"github.com/typesanitizer/happygo/common/check"
	. "github.com/typesanitizer/happygo/common/core"
)

func TestParseSingleRemoteRef(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	const branchRef = "refs/heads/merge-bot/sync/go"

	tests := []struct {
		name    string
		out     string
		want    Option[remoteRef]
		wantErr string
	}{
		{
			name:    "BranchAbsent",
			out:     "",
			want:    None[remoteRef](),
			wantErr: "",
		},
		{
			name: "BranchPresent",
			out:  "0123456789abcdef0123456789abcdef01234567\trefs/heads/merge-bot/sync/go\n",
			want: Some(remoteRef{
				Name: "refs/heads/merge-bot/sync/go",
				SHA:  "0123456789abcdef0123456789abcdef01234567",
			}),
			wantErr: "",
		},
		{
			name:    "WrongRef",
			out:     "0123456789abcdef0123456789abcdef01234567\trefs/heads/merge-bot/sync/tools\n",
			want:    None[remoteRef](),
			wantErr: `expected ls-remote ref "refs/heads/merge-bot/sync/go"`,
		},
		{
			name:    "MalformedLine",
			out:     "0123456789abcdef0123456789abcdef01234567\n",
			want:    None[remoteRef](),
			wantErr: "expected 2 fields in ls-remote output",
		},
		{
			name:    "MultipleLines",
			out:     "a\trefs/heads/merge-bot/sync/go\nb\trefs/heads/merge-bot/sync/go\n",
			want:    None[remoteRef](),
			wantErr: "expected at most 1 ls-remote line",
		},
	}
	for _, tt := range tests {
		h.Run(tt.name, func(h check.Harness) {
			h.Parallel()

			got, err := parseSingleRemoteRef(branchRef, tt.out)
			if tt.wantErr != "" {
				h.Assertf(err != nil, "expected error containing %q, got nil", tt.wantErr)
				h.Assertf(strings.Contains(err.Error(), tt.wantErr),
					"got error %q, want substring %q", err.Error(), tt.wantErr)
				return
			}
			h.NoErrorf(err, "parseSingleRemoteRef")
			h.Assertf(got == tt.want, "got %#v, want %#v", got, tt.want)
		})
	}
}
