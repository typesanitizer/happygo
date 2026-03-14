package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/typesanitizer/happygo/common/logx"
)

const (
	agentLoopImageTag              = "agentloop:local"
	agentLoopClaudeVersion         = "2.1.76"
	agentLoopCodexVersion          = "0.114.0"
	agentLoopContainerfile         = "meta/cmd/agent_loop.Containerfile"
	agentLoopContainerSrcRoot      = "/src-ro"
	agentLoopContainerWorkRoot     = "/sandbox/workspace"
	agentLoopContainerArtifacts    = "/artifacts"
	agentLoopContainerHome         = "/home/agent"
	agentLoopHostConfigRoot        = "/host-config"
	agentLoopContainerArtifactsEnv = "AGENT_LOOP_ARTIFACT_ROOT"
)

type agentLoopPodmanOptions struct {
	Workspace     string
	Artifacts     string
	Image         string
	ClaudeVersion string
	CodexVersion  string
	Network       string
	APIProxy      string
	SkipBuild     bool
}

func newAgentLoopCommands(logger *logx.Logger, getWorkspace func() (Workspace, error)) []*cli.Command {
	return []*cli.Command{
		{
			Name:  "agent-loop",
			Usage: "run the Agent Loop inside a podman sandbox; pass inner loop flags after --",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "workspace"},
				&cli.StringFlag{Name: "artifacts"},
				&cli.StringFlag{Name: "image", Value: agentLoopImageTag},
				&cli.StringFlag{Name: "claude-version", Value: agentLoopClaudeVersion},
				&cli.StringFlag{Name: "codex-version", Value: agentLoopCodexVersion},
				&cli.StringFlag{Name: "network", Value: "none"},
				&cli.StringFlag{Name: "api-proxy"},
				&cli.BoolFlag{Name: "skip-build"},
			},
			Action: func(ctx context.Context, cmd *cli.Command) error {
				ws, err := getWorkspace()
				if err != nil {
					return err
				}
				opts := agentLoopPodmanOptions{
					Workspace:     cmd.String("workspace"),
					Artifacts:     cmd.String("artifacts"),
					Image:         cmd.String("image"),
					ClaudeVersion: cmd.String("claude-version"),
					CodexVersion:  cmd.String("codex-version"),
					Network:       cmd.String("network"),
					APIProxy:      cmd.String("api-proxy"),
					SkipBuild:     cmd.Bool("skip-build"),
				}
				return runAgentLoopViaPodman(ctx, logger, ws.RepoRoot, opts, cmd.Args().Slice())
			},
		},
		{
			Name:   "agent-loop-inner",
			Usage:  "internal container entrypoint for Agent Loop",
			Hidden: true,
			Action: func(ctx context.Context, cmd *cli.Command) error {
				return runAgentLoopInContainer(ctx, logger, cmd.Args().Slice())
			},
		},
	}
}

func runAgentLoopViaPodman(ctx context.Context, logger *logx.Logger, repoRoot string, opts agentLoopPodmanOptions, innerArgs []string) error {
	if err := ensureNoReservedAgentLoopArgs(innerArgs); err != nil {
		return err
	}
	if _, err := exec.LookPath("podman"); err != nil {
		return fmt.Errorf("podman is required but was not found on PATH: %w", err)
	}

	workspace := opts.Workspace
	if workspace == "" {
		workspace = repoRoot
	}
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}

	artifacts, err := resolvePodmanArtifactsDir(workspaceAbs, opts.Artifacts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		return err
	}

	image := opts.Image
	if image == "" {
		image = agentLoopImageTag
	}

	if !opts.SkipBuild {
		buildArgs := []string{
			"build",
			"--build-arg", "CLAUDE_CODE_VERSION=" + opts.ClaudeVersion,
			"--build-arg", "CODEX_VERSION=" + opts.CodexVersion,
			"-t", image,
			"-f", filepath.Join(repoRoot, agentLoopContainerfile),
			repoRoot,
		}
		logger.Info("building podman image", "image", image)
		if err := runStreamingCommand(ctx, "podman", buildArgs...); err != nil {
			return err
		}
	}

	runArgs := []string{
		"run", "--rm", "-t",
		"-v", workspaceAbs + ":" + agentLoopContainerSrcRoot + ":ro",
		"-v", artifacts + ":" + agentLoopContainerArtifacts,
		"-e", "AGENT_LOOP_ARTIFACTS=" + filepath.Join(agentLoopContainerArtifacts, "agentloop"),
		"-e", agentLoopContainerArtifactsEnv + "=" + agentLoopContainerArtifacts,
		"-e", "ANTHROPIC_API_KEY",
		"-e", "OPENAI_API_KEY",
		"-e", "OPENAI_BASE_URL",
		"-e", "ANTHROPIC_BASE_URL",
		"-e", "ANTHROPIC_AUTH_TOKEN",
	}

	if info, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".claude")); err == nil && info.IsDir() {
		runArgs = append(runArgs, "-v", filepath.Join(os.Getenv("HOME"), ".claude")+":"+filepath.Join(agentLoopHostConfigRoot, ".claude")+":ro")
	}
	if info, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".codex")); err == nil && info.IsDir() {
		runArgs = append(runArgs, "-v", filepath.Join(os.Getenv("HOME"), ".codex")+":"+filepath.Join(agentLoopHostConfigRoot, ".codex")+":ro")
	}

	network := opts.Network
	if network == "" {
		network = "none"
	}
	if opts.APIProxy != "" {
		network = "bridge"
		runArgs = append(runArgs,
			"--network", network,
			"-e", "HTTP_PROXY="+opts.APIProxy,
			"-e", "HTTPS_PROXY="+opts.APIProxy,
			"-e", "NO_PROXY=127.0.0.1,localhost,host.containers.internal",
		)
	} else {
		runArgs = append(runArgs, "--network", network)
	}

	runArgs = append(runArgs, image, "-workspace", agentLoopContainerWorkRoot, "-external-sandbox")
	runArgs = append(runArgs, innerArgs...)

	logger.Info("running Agent Loop in podman", "image", image, "workspace", workspaceAbs, "artifacts", artifacts)
	return runStreamingCommand(ctx, "podman", runArgs...)
}

func resolvePodmanArtifactsDir(workspace, artifacts string) (string, error) {
	if artifacts != "" {
		if filepath.IsAbs(artifacts) {
			return artifacts, nil
		}
		return filepath.Abs(filepath.Join(workspace, artifacts))
	}
	name := time.Now().Format("20060102T150405")
	return filepath.Abs(filepath.Join(workspace, ".cache", "tmp", "agentloop-podman", name))
}

func ensureNoReservedAgentLoopArgs(args []string) error {
	for i := 0; i < len(args); i++ {
		if args[i] == "-workspace" || args[i] == "--workspace" || args[i] == "-external-sandbox" || args[i] == "--external-sandbox" {
			return fmt.Errorf("%s is reserved by meta agent-loop and must not be passed after --", args[i])
		}
	}
	return nil
}

func runAgentLoopInContainer(ctx context.Context, logger *logx.Logger, args []string) error {
	artifactRoot := os.Getenv(agentLoopContainerArtifactsEnv)
	if artifactRoot == "" {
		artifactRoot = agentLoopContainerArtifacts
	}
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(agentLoopContainerHome, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(agentLoopContainerWorkRoot, 0o755); err != nil {
		return err
	}

	if err := copyTreeIfExists(filepath.Join(agentLoopHostConfigRoot, ".claude"), filepath.Join(agentLoopContainerHome, ".claude")); err != nil {
		return err
	}
	if err := copyTreeIfExists(filepath.Join(agentLoopHostConfigRoot, ".codex"), filepath.Join(agentLoopContainerHome, ".codex")); err != nil {
		return err
	}

	if err := os.Setenv("HOME", agentLoopContainerHome); err != nil {
		return err
	}
	if err := os.Setenv(sandboxModeEnv, "container"); err != nil {
		return err
	}
	if os.Getenv("AGENT_LOOP_ARTIFACTS") == "" {
		if err := os.Setenv("AGENT_LOOP_ARTIFACTS", filepath.Join(artifactRoot, "agentloop")); err != nil {
			return err
		}
	}

	if _, err := exec.LookPath("claude"); err != nil {
		return err
	}
	if _, err := exec.LookPath("codex"); err != nil {
		return err
	}
	writeCommandOutput(ctx, filepath.Join(artifactRoot, "claude-version.txt"), "claude", "--version")
	writeCommandOutput(ctx, filepath.Join(artifactRoot, "codex-version.txt"), "codex", "--version")

	if err := bootstrapCodexAPIKey(ctx, artifactRoot); err != nil {
		logger.Warn("codex api-key bootstrap failed", "err", err)
	}

	if err := copyTree(agentLoopContainerSrcRoot, agentLoopContainerWorkRoot); err != nil {
		return err
	}

	runErr := runAgentLoopCLI(ctx, args, os.Stdout, os.Stderr)
	writeCommandOutput(ctx, filepath.Join(artifactRoot, "git-status.txt"), "git", "-C", agentLoopContainerWorkRoot, "status", "--short")
	writeCommandOutput(ctx, filepath.Join(artifactRoot, "workspace.patch"), "git", "-C", agentLoopContainerWorkRoot, "diff", "--binary")
	if err := writeTarGz(filepath.Join(artifactRoot, "workspace.tgz"), agentLoopContainerWorkRoot); err != nil {
		logger.Warn("write workspace tarball", "err", err)
	}
	return runErr
}

func bootstrapCodexAPIKey(ctx context.Context, artifactRoot string) error {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return nil
	}
	authFile := filepath.Join(agentLoopContainerHome, ".codex", "auth.json")
	if _, err := os.Stat(authFile); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(authFile), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "codex", "login", "--with-api-key")
	cmd.Stdin = strings.NewReader(apiKey)
	output, err := cmd.CombinedOutput()
	if writeErr := os.WriteFile(filepath.Join(artifactRoot, "codex-login.log"), output, 0o644); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return fmt.Errorf("codex login --with-api-key: %w", err)
	}
	return nil
}

func runStreamingCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeCommandOutput(ctx context.Context, filename string, name string, args ...string) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		output = []byte(err.Error() + "\n")
	}
	_ = os.WriteFile(filename, output, 0o644)
}

func copyTreeIfExists(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return copyTree(src, dst)
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := dst
		if rel != "." {
			target = filepath.Join(dst, rel)
		}
		switch mode := info.Mode(); {
		case info.IsDir():
			return os.MkdirAll(target, mode.Perm())
		case mode&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		case mode.IsRegular():
			return copyFile(path, target, mode.Perm())
		default:
			return nil
		}
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func writeTarGz(filename string, root string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		linkTarget := ""
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		header, err := tar.FileInfoHeader(info, linkTarget)
		if err != nil {
			return err
		}
		header.Name = rel
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(tarWriter, in)
		return err
	})
}
