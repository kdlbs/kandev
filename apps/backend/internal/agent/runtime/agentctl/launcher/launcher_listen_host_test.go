package launcher

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const listenHostEnvPrefix = "AGENTCTL_LISTEN_HOST="

// TestBuildAndStartProcessPinsChildListenHost checks the listen host the
// launcher hands the agentctl child for each shape of agent.standaloneHost.
// It is the host the backend dials, so the control server and its instance
// servers never listen on an interface nothing connects through.
func TestBuildAndStartProcessPinsChildListenHost(t *testing.T) {
	cases := []struct {
		name string
		host string
		want string
	}{
		{name: "default IPv4 loopback", host: "127.0.0.1", want: "127.0.0.1"},
		{name: "empty host uses the launcher default", host: "", want: "localhost"},
		{name: "bracketed IPv6 loopback", host: "[::1]", want: "[::1]"},
		{name: "operator-selected non-loopback host", host: "192.0.2.10", want: "192.0.2.10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := childListenHostEntries(t, tc.host)
			want := []string{listenHostEnvPrefix + tc.want}
			if !slices.Equal(got, want) {
				t.Fatalf("child listen host entries = %q, want %q", got, want)
			}
		})
	}
}

// TestBuildAndStartProcessReplacesInheritedListenHost checks that a listen
// host inherited from the backend's own environment cannot move the child
// away from the host the backend dials.
func TestBuildAndStartProcessReplacesInheritedListenHost(t *testing.T) {
	t.Setenv("AGENTCTL_LISTEN_HOST", "0.0.0.0")

	got := childListenHostEntries(t, "127.0.0.1")
	want := []string{listenHostEnvPrefix + "127.0.0.1"}
	if !slices.Equal(got, want) {
		t.Fatalf("child listen host entries = %q, want %q", got, want)
	}
}

// childListenHostEntries composes the child environment for a launcher bound
// to host and returns its AGENTCTL_LISTEN_HOST entries. The binary path does
// not exist, so process creation fails after the environment is composed:
// nothing is executed and nothing listens.
func childListenHostEntries(t *testing.T, host string) []string {
	t.Helper()
	l := New(Config{
		BinaryPath: filepath.Join(t.TempDir(), "agentctl-not-installed"),
		Host:       host,
	}, newUnexpectedExitTestLogger(t))
	if err := l.buildAndStartProcess("test-nonce"); err == nil {
		_ = l.cmd.Process.Kill()
		t.Fatal("buildAndStartProcess started a binary that does not exist")
	}

	var entries []string
	for _, entry := range l.cmd.Env {
		if strings.HasPrefix(entry, listenHostEnvPrefix) {
			entries = append(entries, entry)
		}
	}
	return entries
}
