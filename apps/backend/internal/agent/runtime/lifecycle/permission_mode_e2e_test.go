package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

const claudeACPTestVersion = "0.81.2"

type permissionModeE2EFrame struct {
	toolName  string
	optionIDs []string
	options   []permissionModeE2EOption
}

type permissionModeE2EOption struct {
	id   string
	kind string
}

type permissionModeE2ERecorder struct {
	mu       sync.Mutex
	frames   []permissionModeE2EFrame
	requests chan permissionModeE2EFrame
	decide   chan string
}

func (r *permissionModeE2ERecorder) handle(ctx context.Context, request *acp.PermissionRequest) (*acp.PermissionResponse, error) {
	frame := permissionModeE2EFrame{}
	if request.ToolName != nil {
		frame.toolName = *request.ToolName
	}
	for _, option := range request.Options {
		frame.optionIDs = append(frame.optionIDs, option.OptionID)
		frame.options = append(frame.options, permissionModeE2EOption{
			id:   option.OptionID,
			kind: string(option.Kind),
		})
	}
	r.mu.Lock()
	r.frames = append(r.frames, frame)
	r.mu.Unlock()
	select {
	case r.requests <- frame:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	// The test answers only after observing the pending request. There is no
	// automatic-approval path in this harness.
	select {
	case optionID := <-r.decide:
		return &acp.PermissionResponse{OptionID: optionID}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *permissionModeE2ERecorder) snapshot() []permissionModeE2EFrame {
	r.mu.Lock()
	defer r.mu.Unlock()
	frames := make([]permissionModeE2EFrame, len(r.frames))
	copy(frames, r.frames)
	return frames
}

type permissionModeE2EBridge struct {
	adapter     *acp.Adapter
	command     *exec.Cmd
	stdin       io.WriteCloser
	info        *acp.AgentInfo
	recorder    *permissionModeE2ERecorder
	updatesDone chan struct{}
}

// TestPermissionModeE2E provisions a temporary HOME and Claude config directory,
// then runs two real Claude ACP 0.81.2 processes concurrently against the same
// disposable Git repository. It never reads the developer's home directory or
// settings. Supply a dedicated API key through
// KANDEV_PERMISSION_MODE_E2E_ANTHROPIC_API_KEY; the child processes receive it
// only as ANTHROPIC_API_KEY. Their stderr and ACP payloads are not printed.
//
// Run with:
//
//	KANDEV_PERMISSION_MODE_E2E=1 KANDEV_PERMISSION_MODE_E2E_ANTHROPIC_API_KEY=<test-key> \
//	  go test ./internal/agent/runtime/lifecycle -run '^TestPermissionModeE2E$' -count=1 -v
//
// A missing key skips this provider-backed check and leaves provider behavior
// unverified. The test does not set IS_SANDBOX: provider restrictions remain
// authoritative on ordinary hosts.
func TestPermissionModeE2E(t *testing.T) {
	if os.Getenv("KANDEV_PERMISSION_MODE_E2E") != "1" {
		t.Skip("set KANDEV_PERMISSION_MODE_E2E=1 to run the isolated Claude provider check")
	}
	apiKey := os.Getenv("KANDEV_PERMISSION_MODE_E2E_ANTHROPIC_API_KEY")
	if strings.TrimSpace(apiKey) == "" {
		t.Skip("provider evidence incomplete: KANDEV_PERMISSION_MODE_E2E_ANTHROPIC_API_KEY is unavailable")
	}
	if _, err := exec.LookPath("npx"); err != nil {
		t.Fatalf("npx is required for Claude ACP %s: %v", claudeACPTestVersion, err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	t.Cleanup(cancel)
	fixtureRoot := t.TempDir()
	isolatedHome := filepath.Join(fixtureRoot, "home")
	configDir := filepath.Join(isolatedHome, ".claude")
	npmCache := filepath.Join(fixtureRoot, "npm-cache")
	tmpDir := filepath.Join(fixtureRoot, "tmp")
	for _, dir := range []string{configDir, npmCache, tmpDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create isolated fixture directory: %v", err)
		}
	}
	for _, path := range []string{filepath.Join(npmCache, "npmrc"), filepath.Join(npmCache, "global-npmrc")} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("write isolated npm config: %v", err)
		}
	}
	settingsPath := filepath.Join(configDir, "settings.json")
	// This isolated fixture represents a stale permission overlay left by an
	// older launch. The session control must select its effective mode without
	// rewriting the file.
	settings := []byte("{\"permissions\":{\"defaultMode\":\"bypassPermissions\"}}\n")
	if err := os.WriteFile(settingsPath, settings, 0o600); err != nil {
		t.Fatalf("write isolated settings fixture: %v", err)
	}
	settingsBefore, err := permissionModeSettingsHash(settingsPath)
	if err != nil {
		t.Fatalf("hash isolated settings fixture: %v", err)
	}

	workDir := filepath.Join(fixtureRoot, "permission-probe-repo")
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatalf("create disposable repository: %v", err)
	}
	if err := permissionModeGit(ctx, isolatedHome, workDir, "init", "--quiet"); err != nil {
		t.Fatalf("initialize disposable repository: %v", err)
	}
	if err := permissionModeGit(ctx, isolatedHome, workDir, "config", "user.name", "Permission Mode E2E"); err != nil {
		t.Fatalf("configure disposable repository identity: %v", err)
	}
	if err := permissionModeGit(ctx, isolatedHome, workDir, "config", "user.email", "permission-mode-e2e@example.invalid"); err != nil {
		t.Fatalf("configure disposable repository identity: %v", err)
	}
	if err := permissionModeGit(ctx, isolatedHome, workDir, "commit", "--allow-empty", "-m", "permission-mode-e2e-baseline"); err != nil {
		t.Fatalf("create disposable repository baseline commit: %v", err)
	}

	childEnv := permissionModeChildEnv(isolatedHome, configDir, npmCache, tmpDir, apiKey)
	defaultBridge := startPermissionModeE2EBridge(t, ctx, "permission-mode-default", workDir, childEnv)
	bypassBridge := startPermissionModeE2EBridge(t, ctx, "permission-mode-bypass", workDir, childEnv)
	for _, bridge := range []*permissionModeE2EBridge{defaultBridge, bypassBridge} {
		if err := bridge.adapter.Initialize(ctx); err != nil {
			t.Fatalf("initialize Claude ACP process: %s", permissionModeRedact(err, apiKey))
		}
		bridge.info = bridge.adapter.GetAgentInfo()
		if _, err := bridge.adapter.NewSession(ctx, nil); err != nil {
			t.Fatalf("create Claude ACP session: %s", permissionModeRedact(err, apiKey))
		}
		if bridge.info == nil || bridge.info.Version == "" || bridge.info.Version == "unknown" {
			t.Fatalf("Claude ACP did not report runtime identity for pinned bridge %s", claudeACPTestVersion)
		}
		t.Logf("bridge=%s runtime=%s/%s executor=local-process isolated-home=true", claudeACPTestVersion, bridge.info.Name, bridge.info.Version)
	}

	defaultMode, err := defaultBridge.adapter.SetMode(ctx, "default")
	if err != nil || !defaultMode.Applied() {
		t.Fatalf("provider did not confirm default mode: result=%+v error=%s", defaultMode, permissionModeRedact(err, apiKey))
	}
	bypassMode, err := bypassBridge.adapter.SetMode(ctx, "bypassPermissions")
	if err != nil {
		after, hashErr := permissionModeSettingsHash(settingsPath)
		if hashErr == nil && after != settingsBefore {
			t.Fatalf("provider denied bypass mode and the isolated settings fixture changed (%s -> %s)", settingsBefore, after)
		}
		t.Skipf("provider evidence incomplete: bypassPermissions was not confirmed; Kandev did not set IS_SANDBOX (%s)", permissionModeRedact(err, apiKey))
	}
	if !bypassMode.Applied() {
		t.Skipf("provider evidence incomplete: bypassPermissions was not confirmed (effective=%q confirmed=%t); Kandev did not set IS_SANDBOX", bypassMode.Effective, bypassMode.Confirmed)
	}
	t.Logf("mode results: default=%+v bypass=%+v", defaultMode, bypassMode)

	const gitMutationPrompt = "Use the Bash tool to run exactly: git commit --allow-empty -m permission-mode-e2e. Do not run any other command or use another tool."
	headBeforeDefault, err := permissionModeGitOutput(ctx, isolatedHome, workDir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("read disposable repository HEAD: %s", permissionModeRedact(err, apiKey))
	}
	defaultPromptDone := make(chan error, 1)
	go func() { defaultPromptDone <- defaultBridge.adapter.Prompt(ctx, gitMutationPrompt, nil, 1) }()
	var defaultRequest permissionModeE2EFrame
	select {
	case defaultRequest = <-defaultBridge.recorder.requests:
	case promptErr := <-defaultPromptDone:
		t.Fatalf("default-mode prompt finished without a permission request (error=%s)", permissionModeRedact(promptErr, apiKey))
	case <-ctx.Done():
		t.Fatalf("default-mode permission request did not arrive: %v", ctx.Err())
	}
	allowOnceID, ok := permissionModeAllowOnceOption(defaultRequest)
	if !ok {
		t.Fatalf("default-mode permission request did not offer allow_once: %v", safePermissionModeFrames([]permissionModeE2EFrame{defaultRequest}))
	}
	headWhilePending, err := permissionModeGitOutput(ctx, isolatedHome, workDir, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(headWhilePending) != strings.TrimSpace(headBeforeDefault) {
		t.Fatalf("Git ref changed while default-mode approval was pending: before=%q pending=%q error=%s", strings.TrimSpace(headBeforeDefault), strings.TrimSpace(headWhilePending), permissionModeRedact(err, apiKey))
	}
	select {
	case promptErr := <-defaultPromptDone:
		t.Fatalf("default-mode prompt finished before explicit approval (error=%s)", permissionModeRedact(promptErr, apiKey))
	case <-time.After(50 * time.Millisecond):
	}
	defaultBridge.recorder.decide <- allowOnceID
	select {
	case promptErr := <-defaultPromptDone:
		if promptErr != nil {
			t.Fatalf("default-mode prompt failed after allow_once: %s", permissionModeRedact(promptErr, apiKey))
		}
	case <-ctx.Done():
		t.Fatalf("default-mode prompt did not finish after allow_once: %v", ctx.Err())
	}
	headAfterDefault, err := permissionModeGitOutput(ctx, isolatedHome, workDir, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(headAfterDefault) == strings.TrimSpace(headBeforeDefault) {
		t.Fatalf("explicitly approved default-mode Git command did not update HEAD: before=%q after=%q error=%s", strings.TrimSpace(headBeforeDefault), strings.TrimSpace(headAfterDefault), permissionModeRedact(err, apiKey))
	}
	if err := permissionModeAssertCommit(ctx, isolatedHome, workDir, strings.TrimSpace(headAfterDefault)); err != nil {
		t.Fatalf("approved default-mode commit is invalid: %s", permissionModeRedact(err, apiKey))
	}

	bypassPromptDone := make(chan error, 1)
	go func() { bypassPromptDone <- bypassBridge.adapter.Prompt(ctx, gitMutationPrompt, nil, 1) }()
	select {
	case bypassRequest := <-bypassBridge.recorder.requests:
		if rejectID, found := permissionModeRejectOnceOption(bypassRequest); found {
			bypassBridge.recorder.decide <- rejectID
		}
		t.Fatalf("bypassPermissions requested approval for Git mutation: %v", safePermissionModeFrames([]permissionModeE2EFrame{bypassRequest}))
	case promptErr := <-bypassPromptDone:
		if promptErr != nil {
			t.Fatalf("bypassPermissions Git prompt failed: %s", permissionModeRedact(promptErr, apiKey))
		}
	case <-ctx.Done():
		t.Fatalf("bypassPermissions prompt did not finish: %v", ctx.Err())
	}
	headAfterBypass, err := permissionModeGitOutput(ctx, isolatedHome, workDir, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(headAfterBypass) == strings.TrimSpace(headAfterDefault) {
		t.Fatalf("bypassPermissions Git command did not update HEAD: before=%q after=%q error=%s", strings.TrimSpace(headAfterDefault), strings.TrimSpace(headAfterBypass), permissionModeRedact(err, apiKey))
	}
	if err := permissionModeAssertCommit(ctx, isolatedHome, workDir, strings.TrimSpace(headAfterBypass)); err != nil {
		t.Fatalf("bypassPermissions commit is invalid: %s", permissionModeRedact(err, apiKey))
	}
	if commits, err := permissionModeGitOutput(ctx, isolatedHome, workDir, "rev-list", "--count", "HEAD"); err != nil || strings.TrimSpace(commits) != "3" {
		t.Fatalf("Git ref history contains %q commits, want baseline plus two verified mutations (error=%s)", strings.TrimSpace(commits), permissionModeRedact(err, apiKey))
	}
	defaultFrames := defaultBridge.recorder.snapshot()
	bypassFrames := bypassBridge.recorder.snapshot()
	t.Logf("permission frames: default=%v bypass=%v", safePermissionModeFrames(defaultFrames), safePermissionModeFrames(bypassFrames))
	settingsAfter, err := permissionModeSettingsHash(settingsPath)
	if err != nil || settingsAfter != settingsBefore {
		t.Fatalf("isolated Claude settings changed: before=%s after=%s error=%s", settingsBefore, settingsAfter, permissionModeRedact(err, apiKey))
	}
	t.Logf("isolated settings sha256 unchanged: %s", settingsAfter)
}

func startPermissionModeE2EBridge(
	t *testing.T,
	ctx context.Context,
	agentID string,
	workDir string,
	childEnv []string,
) *permissionModeE2EBridge {
	t.Helper()
	command := exec.CommandContext(ctx, "npx", "--yes", "@agentclientprotocol/claude-agent-acp@"+claudeACPTestVersion)
	command.Dir = workDir
	command.Env = childEnv
	command.Stderr = io.Discard
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("open Claude ACP stdin: %v", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("open Claude ACP stdout: %v", err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start Claude ACP %s: %v", claudeACPTestVersion, err)
	}
	adapterLogger, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create silent ACP logger: %v", err)
	}
	adapter := acp.NewAdapter(&shared.Config{AgentID: agentID, WorkDir: workDir}, adapterLogger)
	if err := adapter.Connect(stdin, stdout); err != nil {
		_ = command.Process.Kill()
		t.Fatalf("connect Claude ACP stdio: %v", err)
	}
	recorder := &permissionModeE2ERecorder{
		requests: make(chan permissionModeE2EFrame, 4),
		decide:   make(chan string, 4),
	}
	adapter.SetPermissionHandler(recorder.handle)
	updatesDone := make(chan struct{})
	go func() {
		defer close(updatesDone)
		for range adapter.Updates() {
		}
	}()
	bridge := &permissionModeE2EBridge{adapter: adapter, command: command, stdin: stdin, recorder: recorder, updatesDone: updatesDone}
	t.Cleanup(func() {
		_ = adapter.Close()
		_ = stdin.Close()
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
		<-updatesDone
	})
	return bridge
}

func permissionModeChildEnv(home, configDir, npmCache, tmpDir, apiKey string) []string {
	env := make([]string, 0, 12)
	for _, key := range []string{"PATH", "SYSTEMROOT", "WINDIR"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	if runtime.GOOS == "windows" {
		if path, ok := os.LookupEnv("Path"); ok {
			env = append(env, "Path="+path)
		}
	}
	return append(env,
		"HOME="+home,
		"USERPROFILE="+home,
		"CLAUDE_CONFIG_DIR="+configDir,
		"ANTHROPIC_API_KEY="+apiKey,
		"npm_config_cache="+npmCache,
		"npm_config_userconfig="+filepath.Join(npmCache, "npmrc"),
		"npm_config_globalconfig="+filepath.Join(npmCache, "global-npmrc"),
		"TMPDIR="+tmpDir,
		"TEMP="+tmpDir,
		"TMP="+tmpDir,
		"CI=true",
		"NO_COLOR=1",
	)
}

func permissionModeSettingsHash(path string) (string, error) {
	settings, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(settings)
	return hex.EncodeToString(hash[:]), nil
}

func permissionModeGit(ctx context.Context, home, workDir string, args ...string) error {
	_, err := permissionModeGitOutput(ctx, home, workDir, args...)
	return err
}

func permissionModeGitOutput(ctx context.Context, home, workDir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = workDir
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + filepath.Join(home, "empty-gitconfig"),
		"GIT_TEMPLATE_DIR=" + filepath.Join(home, "empty-git-template"),
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}

func permissionModeAssertCommit(ctx context.Context, home, workDir, commit string) error {
	if _, err := permissionModeGitOutput(ctx, home, workDir, "cat-file", "-e", commit+"^{commit}"); err != nil {
		return err
	}
	subject, err := permissionModeGitOutput(ctx, home, workDir, "show", "-s", "--format=%s", commit)
	if err != nil {
		return err
	}
	if strings.TrimSpace(subject) != "permission-mode-e2e" {
		return fmt.Errorf("commit subject = %q, want permission-mode-e2e", strings.TrimSpace(subject))
	}
	return nil
}

func permissionModeAllowOnceOption(frame permissionModeE2EFrame) (string, bool) {
	for _, option := range frame.options {
		if option.kind == "allow_once" {
			return option.id, true
		}
	}
	return "", false
}

func permissionModeRejectOnceOption(frame permissionModeE2EFrame) (string, bool) {
	for _, option := range frame.options {
		if option.kind == "reject_once" {
			return option.id, true
		}
	}
	return "", false
}

func safePermissionModeFrames(frames []permissionModeE2EFrame) []string {
	safe := make([]string, 0, len(frames))
	for _, frame := range frames {
		kinds := make([]string, 0, len(frame.options))
		for _, option := range frame.options {
			kinds = append(kinds, option.kind)
		}
		safe = append(safe, fmt.Sprintf("tool=%q option_ids=%v option_kinds=%v", frame.toolName, frame.optionIDs, kinds))
	}
	return safe
}

func permissionModeRedact(err error, secret string) string {
	if err == nil {
		return "none"
	}
	message := err.Error()
	if secret != "" {
		message = strings.ReplaceAll(message, secret, "[redacted]")
	}
	return message
}
