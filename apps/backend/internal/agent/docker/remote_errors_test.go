package docker

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateRemoteDaemonAddressRejectsSchemes keeps a remote Docker profile
// storing an SSH target rather than a Docker host URL. Rejecting every scheme
// by construction is what excludes tcp:// without maintaining a denylist.
func TestValidateRemoteDaemonAddressRejectsSchemes(t *testing.T) {
	rejected := []string{
		"tcp://build-box:2375",
		"unix:///var/run/docker.sock",
		"npipe:////./pipe/docker_engine",
		"ssh://dev@build-box",
		"http://build-box:2375",
	}
	for _, addr := range rejected {
		if err := ValidateRemoteDaemonAddress(addr); err == nil {
			t.Errorf("ValidateRemoteDaemonAddress(%q) = nil, want error", addr)
		}
	}
}

// TestValidateRemoteDaemonAddressAcceptsHostForms accepts what an SSH target
// actually looks like: a hostname, a user@host, or an OpenSSH config alias.
func TestValidateRemoteDaemonAddressAcceptsHostForms(t *testing.T) {
	accepted := []string{
		"build-box",
		"dev@build-box",
		"build-box.internal",
		"192.0.2.10",
		"prod-alias",
	}
	for _, addr := range accepted {
		if err := ValidateRemoteDaemonAddress(addr); err != nil {
			t.Errorf("ValidateRemoteDaemonAddress(%q) = %v, want nil", addr, err)
		}
	}
}

// TestValidateRemoteDaemonAddressRejectsEmpty fails closed on a blank target
// rather than letting the dialer decide.
func TestValidateRemoteDaemonAddressRejectsEmpty(t *testing.T) {
	for _, addr := range []string{"", "   "} {
		if err := ValidateRemoteDaemonAddress(addr); err == nil {
			t.Errorf("ValidateRemoteDaemonAddress(%q) = nil, want error", addr)
		}
	}
}

// TestClassifyRemoteDialError separates the causes an operator has to act on
// differently. Collapsing these into one "docker unavailable" is the current
// local-executor behavior and is not adequate when the daemon is a network
// hop away: permission, a missing CLI, and a stopped daemon need different
// fixes on a different machine.
func TestClassifyRemoteDialError(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		stderr   string
		want     error
	}{
		{
			name:     "socket permission denied",
			exitCode: 1,
			stderr:   "permission denied while trying to connect to the Docker daemon socket",
			want:     ErrRemoteSocketAccessDenied,
		},
		{
			name:     "socket permission denied, dial variant",
			exitCode: 1,
			stderr:   "dial unix /var/run/docker.sock: connect: permission denied",
			want:     ErrRemoteSocketAccessDenied,
		},
		{
			name:     "docker cli absent",
			exitCode: 127,
			stderr:   "bash: line 1: docker: command not found",
			want:     ErrRemoteDockerCLIMissing,
		},
		{
			name:     "daemon not running",
			exitCode: 1,
			stderr:   "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
			want:     ErrRemoteDaemonUnreachable,
		},
		{
			name:     "unrecognized failure stays generic",
			exitCode: 3,
			stderr:   "something nobody has seen before",
			want:     ErrRemoteDaemonUnreachable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyRemoteDialError(tc.exitCode, tc.stderr)
			if !errors.Is(got, tc.want) {
				t.Fatalf("ClassifyRemoteDialError(%d, %q) = %v, want %v",
					tc.exitCode, tc.stderr, got, tc.want)
			}
		})
	}
}

// TestClassifyRemoteDialErrorPreservesDetail keeps the remote's own words in
// the message so an unrecognized failure is still diagnosable.
func TestClassifyRemoteDialErrorPreservesDetail(t *testing.T) {
	err := ClassifyRemoteDialError(1, "Cannot connect to the Docker daemon at unix:///var/run/docker.sock")
	if err == nil {
		t.Fatal("ClassifyRemoteDialError = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "unix:///var/run/docker.sock") {
		t.Fatalf("error %q dropped the remote detail", got)
	}
}

// TestClassifyRemoteDialErrorSuccess reports no error for a clean exit.
func TestClassifyRemoteDialErrorSuccess(t *testing.T) {
	if err := ClassifyRemoteDialError(0, ""); err != nil {
		t.Fatalf("ClassifyRemoteDialError(0, \"\") = %v, want nil", err)
	}
}
