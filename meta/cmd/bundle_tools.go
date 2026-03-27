package main

import (
	"os"
	"path/filepath"

	"github.com/typesanitizer/happygo/common/cmdx"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/logx"
)

// bundleTools builds all necessary tools from workspace subtrees into go/bin/.
func (ws Workspace) bundleTools(ctx logx.LogCtx) error {
	goBin := filepath.Join(ws.RepoRoot, "go", "bin", "go")
	if _, err := os.Stat(goBin); err != nil {
		return errorx.Newf("nostack", "head toolchain not found at %s; build it first (go/src/make.bash)", goBin)
	}

	outputDir := filepath.Join(ws.RepoRoot, "go", "bin")

	builds := []struct {
		moduleDir string
		pkg       string
		output    string
	}{
		{moduleDir: "delve", pkg: "./cmd/dlv", output: filepath.Join(outputDir, "dlv")},
		{moduleDir: "tools/gopls", pkg: ".", output: filepath.Join(outputDir, "gopls")},
	}

	runOpts := cmdx.RunOptions{CaptureStdout: false, Env: func(env []string) []string {
		return append(env, "GOWORK=off")
	}}
	for _, b := range builds {
		ctx.Info("building", "pkg", b.pkg, "output", b.output)
		cmd := cmdx.New(goBin, "build", "-C", filepath.Join(ws.RepoRoot, b.moduleDir), "-o", b.output, b.pkg)
		if _, err := cmd.Run(ctx, runOpts); err != nil {
			return err
		}
	}

	return nil
}
