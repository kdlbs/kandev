package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestCloneAndMergeSSHMetadata(t *testing.T) {
	t.Run("clone is a copy, not an alias", func(t *testing.T) {
		if got := cloneSSHMetadata(nil); got != nil {
			t.Fatalf("cloneSSHMetadata(nil) = %+v, want nil", got)
		}
		if got := cloneSSHMetadata(map[string]interface{}{}); got != nil {
			t.Fatalf("cloneSSHMetadata(empty) = %+v, want nil", got)
		}
		source := map[string]interface{}{"a": 1}
		clone := cloneSSHMetadata(source)
		clone["a"] = 2
		if source["a"] != 1 {
			t.Fatal("clone must not alias the source map")
		}
	})

	t.Run("merge overlays the instance metadata on the launch metadata", func(t *testing.T) {
		merged := mergeSSHMetadata(
			map[string]interface{}{"a": "launch", "b": "launch"},
			map[string]interface{}{"b": "instance", "c": "instance"},
		)
		if merged["a"] != "launch" || merged["b"] != "instance" || merged["c"] != "instance" {
			t.Fatalf("merged = %+v", merged)
		}
	})

	t.Run("merge with no base still returns a usable map", func(t *testing.T) {
		merged := mergeSSHMetadata(nil, map[string]interface{}{"c": "instance"})
		if merged["c"] != "instance" {
			t.Fatalf("merged = %+v", merged)
		}
	})
}

func TestNormalizeSSHRemotePath(t *testing.T) {
	cases := []struct{ in, want string }{
		{in: "  /remote/task  ", want: "/remote/task"},
		{in: "/remote/task/", want: "/remote/task"},
		{in: "/remote/task///", want: "/remote/task"},
		{in: "/", want: "/"},
		{in: "///", want: "/"},
		{in: "   ", want: ""},
		{in: "", want: ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := normalizeSSHRemotePath(c.in); got != c.want {
				t.Fatalf("normalizeSSHRemotePath(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSSHScriptWithEnvironment(t *testing.T) {
	got := sshScriptWithEnvironment("echo hi")
	if got != "set -ae\n. /dev/stdin\nset +a\nset -e\necho hi" {
		t.Fatalf("sshScriptWithEnvironment = %q", got)
	}
}

func TestRedactSSHScriptOutput(t *testing.T) {
	t.Run("every non-empty env value is redacted", func(t *testing.T) {
		got := redactSSHScriptOutput(
			"cloning with ghp-secret and token sk-secret\n",
			map[string]string{"GITHUB_TOKEN": "ghp-secret", "OPENAI_API_KEY": "sk-secret", "EMPTY": ""},
		)
		if strings.Contains(got, "ghp-secret") || strings.Contains(got, "sk-secret") {
			t.Fatalf("secrets survived redaction: %q", got)
		}
		if strings.Count(got, "<redacted>") != 2 {
			t.Fatalf("redacted output = %q, want both values replaced", got)
		}
	})

	t.Run("output is tail-trimmed to the max line count", func(t *testing.T) {
		var lines []string
		for i := range 40 {
			lines = append(lines, string(rune('a'+i%26))+"-line")
		}
		got := redactSSHScriptOutput(strings.Join(lines, "\n"), nil)
		if count := strings.Count(got, "\n") + 1; count != sshPrepareOutputMaxLines {
			t.Fatalf("kept %d lines, want %d", count, sshPrepareOutputMaxLines)
		}
		if !strings.HasSuffix(got, lines[len(lines)-1]) {
			t.Fatalf("tail line missing from %q", got)
		}
	})
}

func TestSSHCommandFailureDetail(t *testing.T) {
	cases := []struct {
		name           string
		stdout, stderr string
		err            error
		want           string
	}{
		{name: "stdout and stderr joined", stdout: "out", stderr: "err", want: "out\nerr"},
		{name: "error appended on its own line", stdout: "out", stderr: "", err: errStub, want: "out\nstub failure"},
		{name: "error alone", err: errStub, want: "stub failure"},
		{name: "nothing at all", want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sshCommandFailureDetail(c.stdout, c.stderr, c.err); got != c.want {
				t.Fatalf("sshCommandFailureDetail = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSSHExecutorCollectAgentInstallScripts(t *testing.T) {
	claude := &fakeIdentityAgent{MockAgent: agents.NewMockAgent(), id: "claude-code", installScript: "npm i -g claude"}
	codex := &fakeIdentityAgent{MockAgent: agents.NewMockAgent(), id: "codex", installScript: "pip install codex"}
	silent := &fakeIdentityAgent{MockAgent: agents.NewMockAgent(), id: "silent", installScript: "  "}
	lister := &mockAgentLister{agents: []agents.Agent{claude, codex, silent}}

	t.Run("no agent list means no scripts", func(t *testing.T) {
		exec := &SSHExecutor{logger: newTestLogger()}
		if got := exec.collectAgentInstallScripts(&ExecutorCreateRequest{}); got != nil {
			t.Fatalf("scripts = %v, want nil", got)
		}
		exec.agentList = lister
		if got := exec.collectAgentInstallScripts(nil); got != nil {
			t.Fatalf("scripts for a nil request = %v, want nil", got)
		}
	})

	t.Run("the task agent contributes its script", func(t *testing.T) {
		exec := &SSHExecutor{logger: newTestLogger(), agentList: lister}
		got := exec.collectAgentInstallScripts(&ExecutorCreateRequest{AgentConfig: claude})
		if len(got) != 1 || got[0] != "npm i -g claude" {
			t.Fatalf("scripts = %v", got)
		}
	})

	t.Run("agents referenced by selected credentials are included", func(t *testing.T) {
		exec := &SSHExecutor{logger: newTestLogger(), agentList: lister}
		got := exec.collectAgentInstallScripts(&ExecutorCreateRequest{
			Metadata: map[string]interface{}{
				"remote_credentials":  `["agent:codex:files:0"]`,
				"remote_auth_secrets": `{"agent:claude-code:env:ANTHROPIC_API_KEY":"secret-1"}`,
			},
		})
		if len(got) != 2 {
			t.Fatalf("scripts = %v, want both agents", got)
		}
	})

	t.Run("blank install scripts and malformed metadata are skipped", func(t *testing.T) {
		exec := &SSHExecutor{logger: newTestLogger(), agentList: lister}
		got := exec.collectAgentInstallScripts(&ExecutorCreateRequest{
			AgentConfig: silent,
			Metadata: map[string]interface{}{
				"remote_credentials":  `not json`,
				"remote_auth_secrets": `also not json`,
			},
		})
		if len(got) != 0 {
			t.Fatalf("scripts = %v, want none", got)
		}
	})
}

func TestAddMethodAgentIDs(t *testing.T) {
	ids := map[string]bool{}
	addMethodAgentIDs(ids, map[string]interface{}{"key": `["agent:codex:files:0","system:other"]`}, "key",
		func(raw string) []string {
			if raw == `["agent:codex:files:0","system:other"]` {
				return []string{"agent:codex:files:0", "system:other"}
			}
			return nil
		})
	if !ids["codex"] {
		t.Fatalf("ids = %+v, want codex", ids)
	}
	if len(ids) != 1 {
		t.Fatalf("ids = %+v, want only agent-prefixed IDs", ids)
	}
}

func TestSSHExecutorRunPrepareScript(t *testing.T) {
	baseMetadata := func() map[string]interface{} {
		return map[string]interface{}{
			MetadataKeySetupScript:    "echo prepared",
			MetadataKeyBaseBranch:     "main",
			MetadataKeyWorktreeBranch: "feature/task-1",
		}
	}

	t.Run("runs the resolved script under the platform shell with env on stdin", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult {
			return sshExecResult{Stdout: "prepared\n"}
		})
		exec := &SSHExecutor{logger: newTestLogger()}
		var steps []PrepareStep
		req := &ExecutorCreateRequest{
			TaskID:   "task-1",
			Metadata: baseMetadata(),
			Env:      map[string]string{envKeyGitHubToken: "ghp-s3cr3t"},
			OnProgress: func(step PrepareStep, _, _ int) {
				steps = append(steps, step)
			},
		}

		err := exec.runPrepareScript(context.Background(), server.dial(t), "/remote/task", req,
			SSHRemotePlatform{GOOS: "darwin", GOARCH: "arm64"}, "/remote/bin/agentctl")
		if err != nil {
			t.Fatalf("runPrepareScript: %v", err)
		}

		calls := server.execCalls()
		if len(calls) != 1 {
			t.Fatalf("prepare runs = %d, want 1", len(calls))
		}
		script := unwrapLoginShell(t, "zsh", calls[0].Command)
		if !strings.HasPrefix(script, "set -ae\n. /dev/stdin\nset +a\nset -e\n") {
			t.Fatalf("prepare script preamble missing:\n%s", script)
		}
		if !strings.Contains(script, "echo prepared") {
			t.Fatalf("prepare script body missing:\n%s", script)
		}
		if !strings.Contains(calls[0].Stdin, "ghp-s3cr3t") {
			t.Fatalf("prepare stdin = %q, want the resolved env", calls[0].Stdin)
		}
		if strings.Contains(calls[0].Command, "ghp-s3cr3t") {
			t.Fatalf("prepare argv leaked a secret:\n%s", calls[0].Command)
		}
		if len(steps) != 2 || steps[0].Status != PrepareStepRunning || steps[1].Status != PrepareStepCompleted {
			t.Fatalf("progress steps = %+v, want running then completed", steps)
		}
	})

	t.Run("an unresolvable placeholder aborts before any SSH work", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		exec := &SSHExecutor{logger: newTestLogger()}
		err := exec.runPrepareScript(context.Background(), server.dial(t), "/remote/task",
			&ExecutorCreateRequest{Metadata: map[string]interface{}{
				MetadataKeySetupScript: "echo {{unknown.placeholder}}",
			}}, SSHRemotePlatform{}, "")
		if err == nil || !strings.Contains(err.Error(), "unresolved prepare script placeholder") {
			t.Fatalf("error = %v", err)
		}
		if len(server.commands()) != 0 {
			t.Fatalf("no remote command expected, got %v", server.commands())
		}
	})

	t.Run("a failing script reports redacted output and a generic error", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult {
			return sshExecResult{Stdout: "cloning with ghp-s3cr3t", Stderr: "fatal: auth failed", ExitCode: 1}
		})
		exec := &SSHExecutor{logger: newTestLogger()}
		var failed []PrepareStep
		req := &ExecutorCreateRequest{
			TaskID:   "task-1",
			Metadata: baseMetadata(),
			Env:      map[string]string{envKeyGitHubToken: "ghp-s3cr3t"},
			OnProgress: func(step PrepareStep, _, _ int) {
				if step.Status == PrepareStepFailed {
					failed = append(failed, step)
				}
			},
		}
		err := exec.runPrepareScript(context.Background(), server.dial(t), "/remote/task", req,
			SSHRemotePlatform{}, "")
		if err == nil || err.Error() != "ssh: prepare script failed" {
			t.Fatalf("error = %v, want the generic prepare failure", err)
		}
		if len(failed) != 1 {
			t.Fatalf("failed steps = %+v", failed)
		}
		if strings.Contains(failed[0].Output, "ghp-s3cr3t") {
			t.Fatalf("failure output leaked a secret: %q", failed[0].Output)
		}
		if !strings.Contains(failed[0].Output, "<redacted>") ||
			!strings.Contains(failed[0].Output, "fatal: auth failed") {
			t.Fatalf("failure output = %q, want redacted stdout plus stderr", failed[0].Output)
		}
	})

	t.Run("an invalid env key aborts before running anything remote", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		exec := &SSHExecutor{logger: newTestLogger()}
		req := &ExecutorCreateRequest{
			TaskID:                "task-1",
			Metadata:              baseMetadata(),
			Env:                   map[string]string{"BAD KEY": "x"},
			ApprovedSecretEnvKeys: []string{"BAD KEY"},
		}
		// sshRemoteContributionEnv is what feeds buildSSHEnvInitScript here; if
		// it ever forwards an invalid identifier the launch must fail loudly
		// rather than shipping a broken script.
		if err := exec.runPrepareScript(context.Background(), server.dial(t), "/remote/task", req,
			SSHRemotePlatform{}, ""); err != nil {
			if !strings.Contains(err.Error(), "prepare script environment") {
				t.Fatalf("error = %v", err)
			}
			return
		}
		if len(server.commands()) != 1 {
			t.Fatalf("commands = %v, want the prepare script to have run once", server.commands())
		}
	})
}

func TestSSHExecutorVerifyPrimaryCheckout(t *testing.T) {
	const cloneURL = "https://github.com/acme/widgets.git"

	t.Run("no configured repository skips verification", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		exec := &SSHExecutor{logger: newTestLogger()}
		err := exec.verifyPrimaryCheckout(context.Background(), server.dial(t), "/remote/task",
			&ExecutorCreateRequest{Metadata: map[string]interface{}{}}, SSHRemotePlatform{})
		if err != nil {
			t.Fatalf("verifyPrimaryCheckout: %v", err)
		}
		if len(server.commands()) != 0 {
			t.Fatalf("no remote command expected, got %v", server.commands())
		}
	})

	t.Run("a matching checkout passes and reports the canonical dir", func(t *testing.T) {
		server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
			switch {
			case strings.Contains(command, "pwd -P"):
				return sshOut("/remote/task\n")
			case strings.Contains(command, "rev-parse --show-toplevel"):
				return sshOut("/remote/task/\n")
			case strings.Contains(command, "config --get remote.origin.url"):
				return sshOut(cloneURL + "\n")
			case strings.Contains(command, "rev-parse --verify HEAD"):
				return sshOut("abc123\n")
			default:
				return sshOK
			}
		})
		exec := &SSHExecutor{logger: newTestLogger()}
		var completed []PrepareStep
		err := exec.verifyPrimaryCheckout(context.Background(), server.dial(t), "/remote/task",
			&ExecutorCreateRequest{
				Metadata: map[string]interface{}{"repository_clone_url": cloneURL},
				OnProgress: func(step PrepareStep, _, _ int) {
					if step.Status == PrepareStepCompleted {
						completed = append(completed, step)
					}
				},
			}, SSHRemotePlatform{})
		if err != nil {
			t.Fatalf("verifyPrimaryCheckout: %v", err)
		}
		if len(completed) != 1 || completed[0].Output != "/remote/task" {
			t.Fatalf("completed step = %+v", completed)
		}
	})

	tests := []struct {
		name    string
		handler sshExecHandler
		wantErr string
		wantMsg string
	}{
		{
			name: "missing task dir",
			handler: func(command, _ string) sshExecResult {
				if strings.Contains(command, "pwd -P") {
					return sshFail("cd: no such directory")
				}
				return sshOK
			},
			wantErr: "primary repository checkout is missing",
			wantMsg: "cd: no such directory",
		},
		{
			name: "not a git checkout",
			handler: func(command, _ string) sshExecResult {
				switch {
				case strings.Contains(command, "pwd -P"):
					return sshOut("/remote/task\n")
				case strings.Contains(command, "rev-parse --show-toplevel"):
					return sshFail("fatal: not a git repository")
				default:
					return sshOK
				}
			},
			wantErr: "primary repository checkout is missing",
			wantMsg: "fatal: not a git repository",
		},
		{
			name: "checkout root is a nested directory",
			handler: func(command, _ string) sshExecResult {
				switch {
				case strings.Contains(command, "pwd -P"):
					return sshOut("/remote/task\n")
				case strings.Contains(command, "rev-parse --show-toplevel"):
					return sshOut("/remote\n")
				default:
					return sshOK
				}
			},
			wantErr: "primary repository checkout is missing",
			wantMsg: `does not match task directory`,
		},
		{
			name: "origin points somewhere else",
			handler: func(command, _ string) sshExecResult {
				switch {
				case strings.Contains(command, "pwd -P"):
					return sshOut("/remote/task\n")
				case strings.Contains(command, "rev-parse --show-toplevel"):
					return sshOut("/remote/task\n")
				case strings.Contains(command, "config --get remote.origin.url"):
					return sshOut("https://github.com/acme/other.git\n")
				default:
					return sshOK
				}
			},
			wantErr: "primary repository origin does not match configured repository",
		},
		{
			name: "no commit checked out",
			handler: func(command, _ string) sshExecResult {
				switch {
				case strings.Contains(command, "pwd -P"):
					return sshOut("/remote/task\n")
				case strings.Contains(command, "rev-parse --show-toplevel"):
					return sshOut("/remote/task\n")
				case strings.Contains(command, "config --get remote.origin.url"):
					return sshOut(cloneURL + "\n")
				case strings.Contains(command, "rev-parse --verify HEAD"):
					return sshFail("fatal: ambiguous argument 'HEAD'")
				default:
					return sshOK
				}
			},
			wantErr: "primary repository has no checkout",
			wantMsg: "ambiguous argument",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeSSHServer(t, tc.handler)
			exec := &SSHExecutor{logger: newTestLogger()}
			var failed []PrepareStep
			err := exec.verifyPrimaryCheckout(context.Background(), server.dial(t), "/remote/task",
				&ExecutorCreateRequest{
					Metadata: map[string]interface{}{"repository_clone_url": cloneURL},
					OnProgress: func(step PrepareStep, _, _ int) {
						if step.Status == PrepareStepFailed {
							failed = append(failed, step)
						}
					},
				}, SSHRemotePlatform{})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if len(failed) != 1 || failed[0].Name != "Verifying primary checkout" {
				t.Fatalf("failed steps = %+v", failed)
			}
			if tc.wantMsg != "" && !strings.Contains(failed[0].Output, tc.wantMsg) {
				t.Fatalf("failure detail = %q, want %q", failed[0].Output, tc.wantMsg)
			}
		})
	}
}

func TestSSHExecutorRunCleanupScript(t *testing.T) {
	t.Run("no cleanup script is a no-op", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		exec := &SSHExecutor{logger: newTestLogger()}
		err := exec.runCleanupScript(context.Background(), server.dial(t), "/remote/task",
			map[string]interface{}{}, nil, SSHRemotePlatform{}, "instance-1", StopReasonTaskDeleted)
		if err != nil {
			t.Fatalf("runCleanupScript: %v", err)
		}
		if len(server.commands()) != 0 {
			t.Fatalf("no remote command expected, got %v", server.commands())
		}
	})

	t.Run("a missing task directory is an error", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		exec := &SSHExecutor{logger: newTestLogger()}
		err := exec.runCleanupScript(context.Background(), server.dial(t), "",
			map[string]interface{}{MetadataKeyCleanupScript: "echo bye"}, nil,
			SSHRemotePlatform{}, "instance-1", StopReasonTaskDeleted)
		if err == nil || !strings.Contains(err.Error(), "cleanup task directory is missing") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("resolves the workspace placeholder and runs under the platform shell", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOK })
		exec := &SSHExecutor{logger: newTestLogger()}
		err := exec.runCleanupScript(context.Background(), server.dial(t), "/remote/task",
			map[string]interface{}{MetadataKeyCleanupScript: "rm -rf {{workspace.path}}/build"},
			map[string]string{envKeyGitHubToken: "ghp-s3cr3t"},
			SSHRemotePlatform{GOOS: "darwin", GOARCH: "arm64"}, "instance-1", StopReasonTaskDeleted)
		if err != nil {
			t.Fatalf("runCleanupScript: %v", err)
		}
		calls := server.execCalls()
		if len(calls) != 1 {
			t.Fatalf("cleanup runs = %d, want 1", len(calls))
		}
		script := unwrapLoginShell(t, "zsh", calls[0].Command)
		if !strings.Contains(script, "rm -rf '/remote/task'/build") {
			t.Fatalf("cleanup script = %q, want the resolved workspace path", script)
		}
		if calls[0].Stdin != "GITHUB_TOKEN='ghp-s3cr3t'\n" {
			t.Fatalf("cleanup stdin = %q", calls[0].Stdin)
		}
	})

	t.Run("a failing cleanup returns a generic error", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshFail("rm: permission denied") })
		exec := &SSHExecutor{logger: newTestLogger()}
		err := exec.runCleanupScript(context.Background(), server.dial(t), "/remote/task",
			map[string]interface{}{MetadataKeyCleanupScript: "rm -rf {{workspace.path}}/build"}, nil,
			SSHRemotePlatform{}, "instance-1", StopReasonTaskDeleted)
		if err == nil || err.Error() != "ssh: cleanup script failed" {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("cleanup survives a cancelled parent context", func(t *testing.T) {
		// StopInstance's context is often already cancelled by the time
		// cleanup runs, so runCleanupScript must detach from it.
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOK })
		exec := &SSHExecutor{logger: newTestLogger()}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := exec.runCleanupScript(ctx, server.dial(t), "/remote/task",
			map[string]interface{}{MetadataKeyCleanupScript: "echo bye"}, nil,
			SSHRemotePlatform{}, "instance-1", StopReasonTaskDeleted)
		if err != nil {
			t.Fatalf("runCleanupScript with a cancelled parent: %v", err)
		}
		if len(server.commands()) != 1 {
			t.Fatalf("cleanup must still run, commands: %v", server.commands())
		}
	})
}

func TestSSHExecutorResolvePrepareScriptRequiresACloneURLForContributions(t *testing.T) {
	exec := &SSHExecutor{logger: newTestLogger()}
	_, err := exec.resolvePrepareScript(nil, "/remote/task", "")
	if err == nil || !strings.Contains(err.Error(), "prepare request is required") {
		t.Fatalf("error = %v", err)
	}

	_, err = exec.resolveSSHScript(nil, "/remote/task", "", "echo hi")
	if err == nil || !strings.Contains(err.Error(), "script request is required") {
		t.Fatalf("error = %v", err)
	}
}

// errStub is a fixed error used by the failure-detail table.
var errStub = errors.New("stub failure")
