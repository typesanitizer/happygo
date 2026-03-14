// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Agent Loop drives an outer engineering loop over one or more agent CLIs.
// It rotates across backends, applies cooldowns for usage/rate limits, and
// runs validation commands after each successful turn.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const sandboxModeEnv = "AGENT_LOOP_SANDBOX"

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("value must not be empty")
	}
	*m = append(*m, value)
	return nil
}

type options struct {
	workspace        string
	prompt           string
	promptFile       string
	planFile         string
	includeFiles     multiFlag
	checks           multiFlag
	agents           multiFlag
	iterations       int
	agentTimeout     time.Duration
	checkTimeout     time.Duration
	baseCooldown     time.Duration
	maxCooldown      time.Duration
	maxAgentFailures int
	artifacts        string
	stopOnClean      bool
	externalSandbox  bool
	verbose          bool

	claudeBin   string
	claudeModel string
	claudeArgs  multiFlag

	codexBin   string
	codexModel string
	codexArgs  multiFlag
}

type agentKind int

const (
	agentClaude agentKind = iota + 1
	agentCodex
)

type agentSpec struct {
	name      string
	kind      agentKind
	bin       string
	model     string
	extraArgs []string
}

type agentState struct {
	spec          agentSpec
	cooldownUntil time.Time
	retryCount    int
	failures      int
	disabled      bool
}

type agentResult struct {
	summaryPath string
	stdoutPath  string
	stderrPath  string
	summary     string
	combined    string
	err         error
	limited     bool
}

type checkResult struct {
	command string
	logPath string
	output  string
	err     error
}

type feedback struct {
	agent      string
	status     string
	agentNotes string
	checks     []checkResult
}

type eventLogger struct {
	mu sync.Mutex
	f  *os.File
}

type event struct {
	Time      time.Time `json:"time"`
	Kind      string    `json:"kind"`
	Iteration int       `json:"iteration,omitempty"`
	Agent     string    `json:"agent,omitempty"`
	Status    string    `json:"status,omitempty"`
	Message   string    `json:"message,omitempty"`
	Path      string    `json:"path,omitempty"`
}

type runner struct {
	opts       options
	artifacts  string
	basePrompt string
	agents     []*agentState
	nextAgent  int
	logger     *eventLogger
	stdout     io.Writer
	stderr     io.Writer
}

func parseAgentLoopFlags(args []string) (options, error) {
	var opts options
	fs := flag.NewFlagSet("agent-loop", flag.ContinueOnError)
	var usage bytes.Buffer
	fs.SetOutput(&usage)

	opts.workspace = "."
	opts.iterations = 8
	opts.agentTimeout = 20 * time.Minute
	opts.checkTimeout = 10 * time.Minute
	opts.baseCooldown = 30 * time.Second
	opts.maxCooldown = 15 * time.Minute
	opts.maxAgentFailures = 2
	opts.stopOnClean = true
	opts.claudeBin = "claude"
	opts.codexBin = "codex"

	fs.StringVar(&opts.workspace, "workspace", opts.workspace, "workspace root for agent and validation commands")
	fs.StringVar(&opts.prompt, "prompt", "", "base prompt text")
	fs.StringVar(&opts.promptFile, "prompt-file", "", "path to a file whose contents become part of the prompt")
	fs.StringVar(&opts.planFile, "plan-file", "", "path to a plan/design file to include in the prompt")
	fs.Var(&opts.includeFiles, "include-file", "additional file to include in the prompt (repeatable)")
	fs.Var(&opts.checks, "check", "validation command to run after each successful agent turn (repeatable)")
	fs.Var(&opts.agents, "agent", "agent backend to use: claude or codex (repeatable)")
	fs.IntVar(&opts.iterations, "iterations", opts.iterations, "maximum successful agent iterations")
	fs.DurationVar(&opts.agentTimeout, "agent-timeout", opts.agentTimeout, "timeout per agent invocation")
	fs.DurationVar(&opts.checkTimeout, "check-timeout", opts.checkTimeout, "timeout per validation command")
	fs.DurationVar(&opts.baseCooldown, "base-cooldown", opts.baseCooldown, "initial cooldown after a rate limit")
	fs.DurationVar(&opts.maxCooldown, "max-cooldown", opts.maxCooldown, "maximum cooldown after repeated rate limits")
	fs.IntVar(&opts.maxAgentFailures, "max-agent-failures", opts.maxAgentFailures, "disable an agent after this many non-limit failures")
	fs.StringVar(&opts.artifacts, "artifacts", "", "artifact directory (default .cache/tmp/agentloop/<timestamp> under the workspace)")
	fs.BoolVar(&opts.stopOnClean, "stop-on-clean", opts.stopOnClean, "stop after the first clean validation pass")
	fs.BoolVar(&opts.externalSandbox, "external-sandbox", false, "assume the caller already provides an external sandbox")
	fs.BoolVar(&opts.verbose, "v", false, "print verbose progress")

	fs.StringVar(&opts.claudeBin, "claude-bin", opts.claudeBin, "path to the Claude CLI binary")
	fs.StringVar(&opts.claudeModel, "claude-model", "", "Claude model override")
	fs.Var(&opts.claudeArgs, "claude-arg", "additional argument passed to the Claude CLI (repeatable)")

	fs.StringVar(&opts.codexBin, "codex-bin", opts.codexBin, "path to the Codex CLI binary")
	fs.StringVar(&opts.codexModel, "codex-model", "", "Codex model override")
	fs.Var(&opts.codexArgs, "codex-arg", "additional argument passed to the Codex CLI (repeatable)")

	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: agent-loop [flags]")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		text := strings.TrimSpace(usage.String())
		if text == "" {
			return opts, err
		}
		return opts, fmt.Errorf("%w\n%s", err, text)
	}
	if fs.NArg() != 0 {
		return opts, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	return opts, nil
}

func runAgentLoopCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	opts, err := parseAgentLoopFlags(args)
	if err != nil {
		return err
	}
	return run(ctx, opts, stdout, stderr)
}

func run(ctx context.Context, opts options, stdout, stderr io.Writer) error {
	if !opts.externalSandbox {
		return errors.New("direct host execution is unsupported; start agent-loop via meta agent-loop")
	}
	if got := strings.TrimSpace(os.Getenv(sandboxModeEnv)); got != "container" {
		return fmt.Errorf("agent-loop requires %s=container; direct host execution is unsupported", sandboxModeEnv)
	}

	workspace, err := filepath.Abs(opts.workspace)
	if err != nil {
		return err
	}
	opts.workspace = workspace

	if opts.iterations <= 0 {
		return errors.New("iterations must be positive")
	}
	if opts.agentTimeout <= 0 {
		return errors.New("agent-timeout must be positive")
	}
	if opts.checkTimeout <= 0 {
		return errors.New("check-timeout must be positive")
	}
	if opts.baseCooldown <= 0 {
		return errors.New("base-cooldown must be positive")
	}
	if opts.maxCooldown < opts.baseCooldown {
		return errors.New("max-cooldown must be at least base-cooldown")
	}
	if opts.maxAgentFailures <= 0 {
		return errors.New("max-agent-failures must be positive")
	}
	if opts.prompt == "" && opts.promptFile == "" && opts.planFile == "" && len(opts.includeFiles) == 0 {
		return errors.New("at least one of -prompt, -prompt-file, -plan-file, or -include-file must be provided")
	}

	if len(opts.agents) == 0 {
		opts.agents = multiFlag{"claude", "codex"}
	}

	artifacts, err := resolveArtifactsDir(opts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		return err
	}

	logger, err := newEventLogger(filepath.Join(artifacts, "events.jsonl"))
	if err != nil {
		return err
	}
	defer logger.Close()

	basePrompt, err := loadPrompt(workspace, opts)
	if err != nil {
		return err
	}

	agents, err := prepareAgents(opts)
	if err != nil {
		return err
	}

	r := runner{
		opts:       opts,
		artifacts:  artifacts,
		basePrompt: basePrompt,
		agents:     agents,
		logger:     logger,
		stdout:     stdout,
		stderr:     stderr,
	}
	r.logger.Log(event{Time: time.Now(), Kind: "start", Message: fmt.Sprintf("workspace=%s artifacts=%s", workspace, artifacts)})

	return r.loop(ctx)
}

func resolveArtifactsDir(opts options) (string, error) {
	if opts.artifacts != "" {
		if filepath.IsAbs(opts.artifacts) {
			return opts.artifacts, nil
		}
		return filepath.Abs(filepath.Join(opts.workspace, opts.artifacts))
	}
	if env := strings.TrimSpace(os.Getenv("AGENT_LOOP_ARTIFACTS")); env != "" {
		if filepath.IsAbs(env) {
			return env, nil
		}
		return filepath.Abs(filepath.Join(opts.workspace, env))
	}
	name := time.Now().Format("20060102T150405")
	return filepath.Abs(filepath.Join(opts.workspace, ".cache", "tmp", "agentloop", name))
}

func loadPrompt(workspace string, opts options) (string, error) {
	var parts []string
	if text := strings.TrimSpace(opts.prompt); text != "" {
		parts = append(parts, text)
	}
	if opts.promptFile != "" {
		text, err := readFile(workspace, opts.promptFile)
		if err != nil {
			return "", err
		}
		parts = append(parts, formatFileSection("Prompt file", opts.promptFile, text))
	}
	if opts.planFile != "" {
		text, err := readFile(workspace, opts.planFile)
		if err != nil {
			return "", err
		}
		parts = append(parts, formatFileSection("Current design/plan", opts.planFile, text))
	}
	for _, path := range opts.includeFiles {
		text, err := readFile(workspace, path)
		if err != nil {
			return "", err
		}
		parts = append(parts, formatFileSection("Included file", path, text))
	}
	return strings.Join(parts, "\n\n"), nil
}

func readFile(workspace, path string) (string, error) {
	filename := resolvePath(workspace, path)
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return string(data), nil
}

func formatFileSection(label, path, text string) string {
	return fmt.Sprintf("%s: %s\n\n%s", label, path, strings.TrimSpace(text))
}

func resolvePath(workspace, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workspace, path)
}

func prepareAgents(opts options) ([]*agentState, error) {
	var agents []*agentState
	for _, name := range opts.agents {
		var spec agentSpec
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "claude":
			spec = agentSpec{
				name:      "claude",
				kind:      agentClaude,
				bin:       opts.claudeBin,
				model:     opts.claudeModel,
				extraArgs: append([]string(nil), opts.claudeArgs...),
			}
		case "codex":
			spec = agentSpec{
				name:      "codex",
				kind:      agentCodex,
				bin:       opts.codexBin,
				model:     opts.codexModel,
				extraArgs: append([]string(nil), opts.codexArgs...),
			}
		default:
			return nil, fmt.Errorf("unknown agent %q", name)
		}
		if _, err := exec.LookPath(spec.bin); err != nil {
			continue
		}
		agents = append(agents, &agentState{spec: spec})
	}
	if len(agents) == 0 {
		return nil, errors.New("no configured agent binaries were found on PATH")
	}
	return agents, nil
}

func (r *runner) loop(ctx context.Context) error {
	var prior *feedback
	for iter := 1; iter <= r.opts.iterations; {
		agent, wait, err := r.pickAgent(time.Now())
		if err != nil {
			return err
		}
		if agent == nil {
			fmt.Fprintf(r.stderr, "all agents cooling down for %v\n", wait)
			r.logger.Log(event{Time: time.Now(), Kind: "cooldown_wait", Message: wait.String()})
			if err := sleepContext(ctx, wait); err != nil {
				return err
			}
			continue
		}

		iterationDir := filepath.Join(r.artifacts, fmt.Sprintf("iteration-%03d", iter))
		if err := os.MkdirAll(iterationDir, 0o755); err != nil {
			return err
		}

		prompt := r.buildPrompt(iter, prior)
		promptPath := filepath.Join(iterationDir, "prompt.txt")
		if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
			return err
		}

		if r.opts.verbose {
			fmt.Fprintf(r.stderr, "iteration %d/%d with %s\n", iter, r.opts.iterations, agent.spec.name)
		}
		r.logger.Log(event{Time: time.Now(), Kind: "iteration_start", Iteration: iter, Agent: agent.spec.name, Path: promptPath})

		result := r.runAgent(ctx, agent, prompt, iterationDir)
		prior = &feedback{
			agent:      agent.spec.name,
			agentNotes: summarize(result.summary, 24, 8<<10),
		}
		if result.limited {
			cooldown := r.cooldownFor(agent, result.combined)
			agent.cooldownUntil = time.Now().Add(cooldown)
			agent.retryCount++
			prior.status = "rate_limited"
			fmt.Fprintf(r.stderr, "%s rate-limited; cooling down for %v\n", agent.spec.name, cooldown)
			r.logger.Log(event{
				Time:      time.Now(),
				Kind:      "agent_rate_limit",
				Iteration: iter,
				Agent:     agent.spec.name,
				Status:    cooldown.String(),
				Message:   summarize(result.combined, 8, 2048),
				Path:      result.stderrPath,
			})
			continue
		}
		if result.err != nil {
			agent.failures++
			prior.status = "error"
			fmt.Fprintf(r.stderr, "%s failed: %v\n", agent.spec.name, result.err)
			r.logger.Log(event{
				Time:      time.Now(),
				Kind:      "agent_error",
				Iteration: iter,
				Agent:     agent.spec.name,
				Status:    result.err.Error(),
				Message:   summarize(result.combined, 8, 2048),
				Path:      result.stderrPath,
			})
			if agent.failures >= r.opts.maxAgentFailures {
				agent.disabled = true
				r.logger.Log(event{Time: time.Now(), Kind: "agent_disabled", Iteration: iter, Agent: agent.spec.name, Status: "too_many_failures"})
				fmt.Fprintf(r.stderr, "%s disabled after %d failures\n", agent.spec.name, agent.failures)
			}
			iter++
			continue
		}

		agent.failures = 0
		agent.retryCount = 0
		agent.cooldownUntil = time.Time{}
		prior.status = "ok"
		r.logger.Log(event{Time: time.Now(), Kind: "agent_success", Iteration: iter, Agent: agent.spec.name, Path: result.summaryPath})

		checkResults, err := r.runChecks(ctx, iter, iterationDir)
		if err != nil {
			return err
		}
		prior.checks = checkResults
		failed := failedChecks(checkResults)
		if failed == 0 {
			fmt.Fprintf(r.stdout, "iteration %d: clean validation with %s\n", iter, agent.spec.name)
			r.logger.Log(event{Time: time.Now(), Kind: "checks_ok", Iteration: iter, Agent: agent.spec.name})
			if r.opts.stopOnClean {
				return nil
			}
		} else {
			fmt.Fprintf(r.stderr, "iteration %d: %d validation command(s) failed\n", iter, failed)
			r.logger.Log(event{Time: time.Now(), Kind: "checks_failed", Iteration: iter, Agent: agent.spec.name, Status: fmt.Sprintf("%d", failed)})
		}
		iter++
	}

	return errors.New("exhausted iterations without a clean validation pass")
}

func (r *runner) buildPrompt(iter int, prior *feedback) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are continuing an automated engineering loop.\n")
	fmt.Fprintf(&b, "Workspace: %s\n", r.opts.workspace)
	fmt.Fprintf(&b, "Iteration: %d of %d\n\n", iter, r.opts.iterations)
	b.WriteString("Primary objective:\n")
	b.WriteString(strings.TrimSpace(r.basePrompt))
	b.WriteString("\n\n")
	if len(r.opts.checks) > 0 {
		b.WriteString("Validation commands:\n")
		for _, cmd := range r.opts.checks {
			fmt.Fprintf(&b, "- %s\n", cmd)
		}
		b.WriteString("\n")
	}
	if prior != nil {
		b.WriteString("Previous loop result:\n")
		fmt.Fprintf(&b, "- agent: %s\n", prior.agent)
		fmt.Fprintf(&b, "- status: %s\n", prior.status)
		if notes := strings.TrimSpace(prior.agentNotes); notes != "" {
			fmt.Fprintf(&b, "- agent summary:\n%s\n", indentBlock(notes, "  "))
		}
		if failed := failedChecks(prior.checks); failed > 0 {
			b.WriteString("- validation failures:\n")
			for _, check := range prior.checks {
				if check.err == nil {
					continue
				}
				fmt.Fprintf(&b, "  - %s\n", check.command)
				fmt.Fprintf(&b, "%s\n", indentBlock(summarize(check.output, 12, 4096), "    "))
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("Make real edits in the workspace when appropriate.\n")
	b.WriteString("When you finish, summarize the changes and any remaining risk briefly.\n")
	return b.String()
}

func indentBlock(text, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func failedChecks(results []checkResult) int {
	failures := 0
	for _, result := range results {
		if result.err != nil {
			failures++
		}
	}
	return failures
}

func (r *runner) pickAgent(now time.Time) (*agentState, time.Duration, error) {
	var bestWait time.Duration = -1
	n := len(r.agents)
	for i := 0; i < n; i++ {
		idx := (r.nextAgent + i) % n
		agent := r.agents[idx]
		if agent.disabled {
			continue
		}
		if now.Before(agent.cooldownUntil) {
			wait := agent.cooldownUntil.Sub(now)
			if bestWait < 0 || wait < bestWait {
				bestWait = wait
			}
			continue
		}
		r.nextAgent = (idx + 1) % n
		return agent, 0, nil
	}
	if bestWait >= 0 {
		return nil, bestWait, nil
	}
	return nil, 0, errors.New("all agents are disabled")
}

func (r *runner) runAgent(ctx context.Context, agent *agentState, prompt string, iterationDir string) agentResult {
	stdoutPath := filepath.Join(iterationDir, agent.spec.name+".stdout.log")
	stderrPath := filepath.Join(iterationDir, agent.spec.name+".stderr.log")
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return agentResult{err: err}
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return agentResult{err: err}
	}
	defer stderrFile.Close()

	cmd, summaryPath, input, err := buildAgentCommand(agent.spec, r.opts.workspace, iterationDir, r.opts.externalSandbox, prompt)
	if err != nil {
		return agentResult{err: err}
	}
	ctx, cancel := context.WithTimeout(ctx, r.opts.agentTimeout)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	cmd.Dir = r.opts.workspace
	cmd.Env = os.Environ()
	cmd.Stdin = strings.NewReader(input)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(stdoutFile, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(stderrFile, &stderrBuf)
	runErr := cmd.Run()

	combined := stdoutBuf.String()
	if stderrBuf.Len() > 0 {
		if combined != "" {
			combined += "\n"
		}
		combined += stderrBuf.String()
	}

	summary := strings.TrimSpace(stdoutBuf.String())
	if summaryPath != "" {
		if data, err := os.ReadFile(summaryPath); err == nil && strings.TrimSpace(string(data)) != "" {
			summary = strings.TrimSpace(string(data))
		}
	}

	result := agentResult{
		summaryPath: summaryPath,
		stdoutPath:  stdoutPath,
		stderrPath:  stderrPath,
		summary:     summary,
		combined:    combined,
		err:         runErr,
	}
	if ctx.Err() == context.DeadlineExceeded {
		result.err = fmt.Errorf("timed out after %v", r.opts.agentTimeout)
	}
	result.limited = isUsageLimited(combined)
	return result
}

func buildAgentCommand(spec agentSpec, workspace, iterationDir string, externalSandbox bool, prompt string) (*exec.Cmd, string, string, error) {
	switch spec.kind {
	case agentClaude:
		args := []string{"-p", "--add-dir", workspace, "--permission-mode", "dontAsk", "--no-session-persistence", "--output-format", "text"}
		if spec.model != "" {
			args = append(args, "--model", spec.model)
		}
		if externalSandbox {
			args = append(args, "--dangerously-skip-permissions")
		}
		args = append(args, spec.extraArgs...)
		args = append(args, prompt)
		return exec.Command(spec.bin, args...), "", "", nil
	case agentCodex:
		lastMessage := filepath.Join(iterationDir, "codex-last-message.txt")
		args := []string{"exec", "--cd", workspace, "--output-last-message", lastMessage, "--skip-git-repo-check"}
		if spec.model != "" {
			args = append(args, "--model", spec.model)
		}
		if externalSandbox {
			args = append(args, "--dangerously-bypass-approvals-and-sandbox")
		} else {
			args = append(args, "--full-auto")
		}
		args = append(args, spec.extraArgs...)
		args = append(args, "-")
		return exec.Command(spec.bin, args...), lastMessage, prompt, nil
	default:
		return nil, "", "", errors.New("unknown agent kind")
	}
}

func (r *runner) cooldownFor(agent *agentState, output string) time.Duration {
	if wait, ok := parseRetryAfter(output); ok {
		if wait < r.opts.baseCooldown {
			wait = r.opts.baseCooldown
		}
		if wait > r.opts.maxCooldown {
			wait = r.opts.maxCooldown
		}
		return wait
	}
	wait := r.opts.baseCooldown
	for i := 0; i < agent.retryCount; i++ {
		wait *= 2
		if wait >= r.opts.maxCooldown {
			return r.opts.maxCooldown
		}
	}
	if wait > r.opts.maxCooldown {
		return r.opts.maxCooldown
	}
	return wait
}

func (r *runner) runChecks(ctx context.Context, iter int, iterationDir string) ([]checkResult, error) {
	if len(r.opts.checks) == 0 {
		return nil, nil
	}
	results := make([]checkResult, 0, len(r.opts.checks))
	for i, check := range r.opts.checks {
		logPath := filepath.Join(iterationDir, fmt.Sprintf("check-%02d.log", i+1))
		ctx, cancel := context.WithTimeout(ctx, r.opts.checkTimeout)
		output, err := runShellCommand(ctx, r.opts.workspace, check)
		cancel()
		if writeErr := os.WriteFile(logPath, output, 0o644); writeErr != nil {
			return nil, writeErr
		}
		result := checkResult{
			command: check,
			logPath: logPath,
			output:  string(output),
			err:     err,
		}
		results = append(results, result)
		status := "ok"
		if err != nil {
			status = err.Error()
		}
		r.logger.Log(event{
			Time:      time.Now(),
			Kind:      "check",
			Iteration: iter,
			Status:    status,
			Message:   summarize(string(output), 8, 2048),
			Path:      logPath,
		})
	}
	return results, nil
}

func runShellCommand(ctx context.Context, dir, command string) ([]byte, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func summarize(text string, maxLines int, maxBytes int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if len(text) > maxBytes {
		text = text[:maxBytes] + "\n..."
	}
	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], "...")
	}
	return strings.Join(lines, "\n")
}

var retryAfterDuration = regexp.MustCompile(`(?i)(?:retry|try again|available)\s+(?:in|after)\s+([0-9]+(?:\.[0-9]+)?(?:ms|s|m|h))`)
var retryAfterWords = regexp.MustCompile(`(?i)(\d+)\s*(second|minute|hour)s?`)

func parseRetryAfter(text string) (time.Duration, bool) {
	if match := retryAfterDuration.FindStringSubmatch(text); len(match) == 2 {
		wait, err := time.ParseDuration(match[1])
		if err == nil {
			return wait, true
		}
	}
	if match := retryAfterWords.FindStringSubmatch(text); len(match) == 3 {
		value := match[1]
		unit := strings.ToLower(match[2])
		switch unit {
		case "second":
			wait, err := time.ParseDuration(value + "s")
			return wait, err == nil
		case "minute":
			wait, err := time.ParseDuration(value + "m")
			return wait, err == nil
		case "hour":
			wait, err := time.ParseDuration(value + "h")
			return wait, err == nil
		}
	}
	return 0, false
}

var usageLimitPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b429\b`),
	regexp.MustCompile(`(?i)too many requests`),
	regexp.MustCompile(`(?i)rate limit`),
	regexp.MustCompile(`(?i)usage limit`),
	regexp.MustCompile(`(?i)retry after`),
	regexp.MustCompile(`(?i)try again in`),
	regexp.MustCompile(`(?i)quota`),
	regexp.MustCompile(`(?i)temporarily unavailable`),
	regexp.MustCompile(`(?i)model is overloaded`),
}

func isUsageLimited(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, pattern := range usageLimitPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func newEventLogger(path string) (*eventLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &eventLogger{f: f}, nil
}

func (l *eventLogger) Log(ev event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	l.f.Write(data)
	l.f.Write([]byte("\n"))
}

func (l *eventLogger) Close() error {
	return l.f.Close()
}
