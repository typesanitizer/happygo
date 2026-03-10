package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/typesanitizer/happygo/common/check"
	"github.com/typesanitizer/happygo/common/logx"
	"github.com/typesanitizer/happygo/meta/internal/config"
)

func TestList(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	root := t.TempDir()
	h.WriteTree(root, map[string]string{
		"alpha/go.mod": "module alpha\n",
		"beta/go.mod":  "module beta\n",
		"gamma/go.mod": "module gamma\n",
		"delta/":       "",
		"file.txt":     "not a dir\n",
	})

	ws := Workspace{
		RepoRoot: root,
		Config: config.WorkspaceConfig{
			ForkedFolders: map[string]config.ForkedFolder{
				"beta": {Folder: "beta", GitHubRepo: "example/beta"},
			},
			BranchMappings: config.BranchMappings{ByLocalBranch: nil},
		},
	}

	tests := []struct {
		name       string
		provenance ListProvenance
		want       string
	}{
		{"All", ListProvenance_All, "alpha\nbeta\ngamma\n"},
		{"FirstParty", ListProvenance_FirstParty, "alpha\ngamma\n"},
		{"Forked", ListProvenance_Forked, "beta\n"},
	}
	for _, tt := range tests {
		h.Run(tt.name, func(h check.Harness) {
			h.Parallel()
			var buf bytes.Buffer
			logger := logx.NewLogger(os.Stderr, logx.ColorSupport_Disable)
			err := ws.List(logger, &buf, ListOptions{Type: ListType_GoModules, Provenance: tt.provenance})
			h.NoErrorf(err, "List(%v)", tt.provenance)
			h.Assertf(buf.String() == tt.want, "got %q, want %q", buf.String(), tt.want)
		})
	}
}
