package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want time.Duration
		ok   bool
	}{
		{text: "429 Too Many Requests. Try again in 45s.", want: 45 * time.Second, ok: true},
		{text: "Please retry after 2m", want: 2 * time.Minute, ok: true},
		{text: "usage limit resets in 3 hours", want: 3 * time.Hour, ok: true},
		{text: "plain failure", want: 0, ok: false},
	}

	for _, test := range tests {
		got, ok := parseRetryAfter(test.text)
		if ok != test.ok || got != test.want {
			t.Fatalf("parseRetryAfter(%q) = (%v, %v), want (%v, %v)", test.text, got, ok, test.want, test.ok)
		}
	}
}

func TestBuildAgentCommandExternalSandbox(t *testing.T) {
	t.Parallel()

	claude, _, _, err := buildAgentCommand(agentSpec{name: "claude", kind: agentClaude, bin: "claude"}, "/tmp/work", "/tmp/iter", true, "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(claude.Args, " "); !strings.Contains(got, "--dangerously-skip-permissions") {
		t.Fatalf("claude args %q missing external sandbox flag", got)
	}

	codex, _, _, err := buildAgentCommand(agentSpec{name: "codex", kind: agentCodex, bin: "codex"}, "/tmp/work", "/tmp/iter", true, "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(codex.Args, " "); !strings.Contains(got, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("codex args %q missing external sandbox flag", got)
	}
}

func TestRunRequiresSandbox(t *testing.T) {
	t.Parallel()

	opts := options{
		workspace:        t.TempDir(),
		prompt:           "make progress",
		agents:           multiFlag{"claude"},
		iterations:       1,
		agentTimeout:     5 * time.Second,
		checkTimeout:     5 * time.Second,
		baseCooldown:     10 * time.Millisecond,
		maxCooldown:      50 * time.Millisecond,
		maxAgentFailures: 1,
		artifacts:        t.TempDir(),
	}

	err := run(context.Background(), opts, ioDiscard{}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "direct host execution is unsupported") {
		t.Fatalf("run() error = %v, want direct-host rejection", err)
	}
}

func TestLoopFallsBackAfterRateLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell scripts")
	}

	t.Setenv(sandboxModeEnv, "container")

	workspace := t.TempDir()
	artifacts := t.TempDir()
	scripts := t.TempDir()

	claude := filepath.Join(scripts, "claude")
	writeExecutable(t, claude, "#!/bin/sh\n"+
		"echo '429 Too Many Requests. Try again in 20ms.' >&2\n"+
		"exit 1\n")

	codex := filepath.Join(scripts, "codex")
	writeExecutable(t, codex, "#!/bin/sh\n"+
		"if [ \"$1\" = \"exec\" ]; then shift; fi\n"+
		"cat >/dev/null\n"+
		"touch \"$PWD/success.txt\"\n"+
		"echo 'codex ok'\n")

	opts := options{
		workspace:        workspace,
		prompt:           "make progress",
		agents:           multiFlag{"claude", "codex"},
		iterations:       1,
		agentTimeout:     5 * time.Second,
		checkTimeout:     5 * time.Second,
		baseCooldown:     10 * time.Millisecond,
		maxCooldown:      50 * time.Millisecond,
		maxAgentFailures: 1,
		artifacts:        artifacts,
		stopOnClean:      true,
		externalSandbox:  true,
		claudeBin:        claude,
		codexBin:         codex,
	}

	if err := run(context.Background(), opts, ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workspace, "success.txt")); err != nil {
		t.Fatalf("expected codex fallback to modify the workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(artifacts, "iteration-001", "claude.stderr.log")); err != nil {
		t.Fatalf("missing claude stderr log: %v", err)
	}
	if _, err := os.Stat(filepath.Join(artifacts, "iteration-001", "codex.stdout.log")); err != nil {
		t.Fatalf("missing codex stdout log: %v", err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
