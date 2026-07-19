package lifecycle

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

type terminalBrokerProcessClient struct {
	exitCode int
	status   agentctl.ProcessStatus
	request  agentctl.StartProcessRequest
}

func (c *terminalBrokerProcessClient) StartProcess(
	_ context.Context,
	req agentctl.StartProcessRequest,
) (*agentctl.ProcessInfo, error) {
	c.request = req
	return &agentctl.ProcessInfo{
		ID: "probe-1", Status: c.status, ExitCode: &c.exitCode,
	}, nil
}

func (c *terminalBrokerProcessClient) GetProcess(
	context.Context,
	string,
	bool,
) (*agentctl.ProcessInfo, error) {
	return nil, errors.New("terminal process must not be polled")
}

func TestBrokerReachabilityPreflightFailsClosedFromExecutorRunner(t *testing.T) {
	env := map[string]string{
		envKeyGitHubCredentialBrokerURL: "https://unreachable.example/resolve",
		envKeyGitHubCredentialLease:     "secret-lease",
	}
	runnerErr := errors.New("network unreachable")
	err := runBrokerReachabilityPreflight(context.Background(), env, func(
		_ context.Context,
		command string,
		probeEnv map[string]string,
	) ([]byte, error) {
		if !strings.Contains(command, "curl") {
			t.Fatalf("probe command = %q", command)
		}
		if probeEnv[envKeyGitHubCredentialBrokerURL] != env[envKeyGitHubCredentialBrokerURL] {
			t.Fatalf("probe URL env = %#v", probeEnv)
		}
		if len(probeEnv) != 1 || strings.Contains(command, "secret-lease") {
			t.Fatalf("probe leaked broker credential: command=%q env=%#v", command, probeEnv)
		}
		return []byte("curl: connection refused"), runnerErr
	})
	if !errors.Is(err, ErrGitHubCredentialBrokerUnreachable) {
		t.Fatalf("preflight error = %v, want unreachable", err)
	}
}

func TestBrokerReachabilityPreflightAllowsReachableAndExplicitTokenExecutors(t *testing.T) {
	calls := 0
	reachable := map[string]string{
		envKeyGitHubCredentialBrokerURL: "https://reachable.example/resolve",
		envKeyGitHubCredentialLease:     "lease",
	}
	if err := runBrokerReachabilityPreflight(context.Background(), reachable,
		func(context.Context, string, map[string]string) ([]byte, error) {
			calls++
			return nil, nil
		}); err != nil {
		t.Fatalf("reachable preflight error = %v", err)
	}
	if err := runBrokerReachabilityPreflight(context.Background(), map[string]string{
		"GITHUB_TOKEN": "explicit-profile-token",
	}, func(context.Context, string, map[string]string) ([]byte, error) {
		calls++
		return nil, errors.New("must not run")
	}); err != nil {
		t.Fatalf("explicit token preflight error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("runner calls = %d, want 1", calls)
	}
}

func TestBrokerReachabilityPreflightRequiresExactReadyResponse(t *testing.T) {
	binDir := t.TempDir()
	curlPath := filepath.Join(binDir, "curl")
	if err := os.WriteFile(curlPath, []byte("#!/bin/sh\nprintf '%s' \"$FAKE_CURL_STATUS\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		envKeyGitHubCredentialBrokerURL: "https://kandev.example/api/v1/github/credentials/resolve",
		envKeyGitHubCredentialLease:     "lease-must-not-be-forwarded",
	}
	for _, test := range []struct {
		name   string
		status string
		wantOK bool
	}{
		{name: "ready", status: "204", wantOK: true},
		{name: "wrong route", status: "404"},
		{name: "broker failure", status: "500"},
		{name: "redirect", status: "301"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := runBrokerReachabilityPreflight(context.Background(), env, func(
				ctx context.Context,
				command string,
				probeEnv map[string]string,
			) ([]byte, error) {
				cmd := exec.CommandContext(ctx, "sh", "-c", command)
				cmd.Env = append(os.Environ(),
					"PATH="+binDir+":"+os.Getenv("PATH"),
					envKeyGitHubCredentialBrokerURL+"="+probeEnv[envKeyGitHubCredentialBrokerURL],
					"FAKE_CURL_STATUS="+test.status,
				)
				return cmd.CombinedOutput()
			})
			if test.wantOK && err != nil {
				t.Fatalf("preflight error = %v", err)
			}
			if !test.wantOK && !errors.Is(err, ErrGitHubCredentialBrokerUnreachable) {
				t.Fatalf("preflight error = %v, want unreachable", err)
			}
		})
	}
}

func TestBrokerReachabilityViaAgentctlUsesExecutorProcessResult(t *testing.T) {
	env := map[string]string{
		envKeyGitHubCredentialBrokerURL: "https://kandev.example/resolve",
		envKeyGitHubCredentialLease:     "lease-must-not-be-forwarded",
	}
	reachable := &terminalBrokerProcessClient{status: agentctltypes.ProcessStatusExited}
	if err := runBrokerReachabilityViaAgentctl(context.Background(), reachable, "session-1", env); err != nil {
		t.Fatalf("reachable agentctl preflight error = %v", err)
	}
	if len(reachable.request.Env) != 1 || reachable.request.Env[envKeyGitHubCredentialBrokerURL] == "" {
		t.Fatalf("agentctl probe env = %#v", reachable.request.Env)
	}

	unreachable := &terminalBrokerProcessClient{status: agentctltypes.ProcessStatusFailed, exitCode: 7}
	err := runBrokerReachabilityViaAgentctl(context.Background(), unreachable, "session-1", env)
	if !errors.Is(err, ErrGitHubCredentialBrokerUnreachable) {
		t.Fatalf("unreachable agentctl preflight error = %v", err)
	}
}
