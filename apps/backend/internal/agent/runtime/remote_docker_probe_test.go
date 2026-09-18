package runtime

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

// probeTestUser keeps the fixture independent of the ambient environment:
// target resolution falls back to $USER, which a CI runner does not set.
const probeTestUser = "builder"

func stubProber() *RemoteDockerProber {
	return &RemoteDockerProber{
		Dial: func(context.Context, *lifecycle.SSHTarget) (*ssh.Client, string, error) {
			return nil, "SHA256:abc123", nil
		},
		Probe: func(context.Context, *ssh.Client) (*lifecycle.SSHRemoteInfo, error) {
			return &lifecycle.SSHRemoteInfo{
				Platform: lifecycle.SSHRemotePlatform{GOOS: "linux", GOARCH: "arm64"},
			}, nil
		},
		Daemon: func(context.Context, *ssh.Client) (string, error) { return "1.51", nil },
	}
}

func stepByName(result RemoteDockerTestResult, name string) (RemoteDockerTestStep, bool) {
	for _, s := range result.Steps {
		if s.Name == name {
			return s, true
		}
	}
	return RemoteDockerTestStep{}, false
}

// TestProbeReportsEveryStep gives the user one row per thing that can fail,
// because each one needs a different fix on the remote host.
func TestProbeReportsEveryStep(t *testing.T) {
	result := stubProber().Run(context.Background(), RemoteDockerTestRequest{Host: "build-box", User: probeTestUser})

	if !result.Success {
		t.Fatalf("Run = failure: %+v", result)
	}
	for _, want := range []string{stepConnect, stepPlatform, stepDaemon, stepAPIVersion} {
		step, ok := stepByName(result, want)
		if !ok {
			t.Errorf("missing step %q; steps = %+v", want, result.Steps)
			continue
		}
		if !step.Success {
			t.Errorf("step %q failed: %s", want, step.Error)
		}
	}
	if result.Fingerprint != "SHA256:abc123" {
		t.Errorf("Fingerprint = %q, want the observed host key", result.Fingerprint)
	}
	if result.Platform != "linux/arm64" {
		t.Errorf("Platform = %q, want linux/arm64", result.Platform)
	}
	if result.DaemonAPIVersion != "1.51" {
		t.Errorf("DaemonAPIVersion = %q, want 1.51", result.DaemonAPIVersion)
	}
}

// TestProbeRejectsDaemonURL keeps a Docker host URL out of the host field
// before anything is dialed.
func TestProbeRejectsDaemonURL(t *testing.T) {
	for _, host := range []string{"tcp://build-box:2375", "ssh://dev@build-box"} {
		result := stubProber().Run(context.Background(), RemoteDockerTestRequest{Host: host, User: probeTestUser})
		if result.Success {
			t.Errorf("Run with host %q succeeded, want rejection", host)
		}
		if result.Error == "" {
			t.Errorf("Run with host %q reported no error", host)
		}
	}
}

// TestProbeSeparatesSocketDenialFromDeadDaemon is the distinction that matters
// most on a first run. A denied socket means the SSH user needs Docker access;
// a dead daemon means the host needs Docker started. Collapsing them sends the
// user to the wrong fix.
func TestProbeSeparatesSocketDenialFromDeadDaemon(t *testing.T) {
	tests := []struct {
		name      string
		daemonErr error
		wantHint  string
	}{
		{
			name:      "socket access denied",
			daemonErr: docker.ErrRemoteSocketAccessDenied,
			wantHint:  hintSocketAccess,
		},
		{
			name:      "docker cli missing",
			daemonErr: docker.ErrRemoteDockerCLIMissing,
			wantHint:  hintDockerCLIMissing,
		},
		{
			name:      "daemon unreachable",
			daemonErr: docker.ErrRemoteDaemonUnreachable,
			wantHint:  hintDaemonUnreachable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := stubProber()
			p.Daemon = func(context.Context, *ssh.Client) (string, error) { return "", tc.daemonErr }

			result := p.Run(context.Background(), RemoteDockerTestRequest{Host: "build-box", User: probeTestUser})
			if result.Success {
				t.Fatal("Run succeeded despite a daemon failure")
			}
			step, ok := stepByName(result, stepDaemon)
			if !ok {
				t.Fatalf("no %q step; steps = %+v", stepDaemon, result.Steps)
			}
			if step.Success {
				t.Fatal("daemon step reported success")
			}
			if step.Hint != tc.wantHint {
				t.Fatalf("hint = %q, want %q", step.Hint, tc.wantHint)
			}
		})
	}
}

// TestProbeStopsAtTheFirstFailure avoids reporting a green daemon under a red
// connection, which would tell the user the opposite of what is wrong.
func TestProbeStopsAtTheFirstFailure(t *testing.T) {
	p := stubProber()
	p.Dial = func(context.Context, *lifecycle.SSHTarget) (*ssh.Client, string, error) {
		return nil, "", errors.New("connection refused")
	}

	result := p.Run(context.Background(), RemoteDockerTestRequest{Host: "build-box", User: probeTestUser})
	if result.Success {
		t.Fatal("Run succeeded with a failed connection")
	}
	if _, ok := stepByName(result, stepDaemon); ok {
		t.Fatal("daemon step ran after the connection failed")
	}
	connect, ok := stepByName(result, stepConnect)
	if !ok || connect.Success {
		t.Fatalf("connect step missing or green: %+v", result.Steps)
	}
}

// TestProbeRejectsUnsupportedPlatform fails before the daemon step, because an
// unsupported architecture cannot run the agentctl helper regardless of Docker.
func TestProbeRejectsUnsupportedPlatform(t *testing.T) {
	p := stubProber()
	p.Probe = func(context.Context, *ssh.Client) (*lifecycle.SSHRemoteInfo, error) {
		return &lifecycle.SSHRemoteInfo{
			Platform: lifecycle.SSHRemotePlatform{GOOS: "plan9", GOARCH: "mips"},
			OS:       "Plan9",
			Arch:     "mips",
		}, nil
	}

	result := p.Run(context.Background(), RemoteDockerTestRequest{Host: "build-box", User: probeTestUser})
	if result.Success {
		t.Fatal("Run succeeded on an unsupported platform")
	}
	step, ok := stepByName(result, stepPlatform)
	if !ok || step.Success {
		t.Fatalf("platform step missing or green: %+v", result.Steps)
	}
	if _, ok := stepByName(result, stepDaemon); ok {
		t.Fatal("daemon step ran after an unsupported platform")
	}
}

// TestProbeRequiresAHost fails closed rather than dialing something implied.
func TestProbeRequiresAHost(t *testing.T) {
	result := stubProber().Run(context.Background(), RemoteDockerTestRequest{})
	if result.Success {
		t.Fatal("Run with no host succeeded")
	}
	if len(result.Steps) != 0 {
		t.Fatalf("steps ran without a host: %+v", result.Steps)
	}
}
