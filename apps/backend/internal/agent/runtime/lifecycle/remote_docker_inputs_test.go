package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
)

// recordingArchiveWriter captures what the runtime hands the daemon, so the
// delivered archive can be read back entry by entry.
type recordingArchiveWriter struct {
	calls []archiveCall
	err   error
}

type archiveCall struct {
	containerID string
	dstPath     string
	archive     []byte
}

func (w *recordingArchiveWriter) CopyToContainer(_ context.Context, containerID, dstPath string, archive []byte) error {
	w.calls = append(w.calls, archiveCall{containerID: containerID, dstPath: dstPath, archive: archive})
	return w.err
}

func (w *recordingArchiveWriter) entries(t *testing.T) []tarEntry {
	t.Helper()
	require.Len(t, w.calls, 1, "expected exactly one archive delivery")
	return readTarEntries(t, w.calls[0].archive)
}

func newRemoteInputsForTest(t *testing.T, w *recordingArchiveWriter) *remoteContainerInputs {
	t.Helper()
	inputs := newRemoteContainerInputs(w, SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"},
		NewCommandBuilder(), newTestLogger())
	inputs.resolveAgentctl = func(SSHRemotePlatform) ([]byte, error) { return []byte("AGENTCTL-ELF"), nil }
	inputs.resolveMockAgentBinary = func() (string, error) { return "", nil }
	return inputs
}

// seedLocalCredential writes a credential file into a fake host home and
// returns an agent that declares it.
func seedLocalCredential(t *testing.T, body string) agents.Agent {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".credential-agent", "creds.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return newCredentialAgent(".credential-agent/creds.json")
}

// TestRemoteContainerMountsNothing is the requirement in one assertion: a
// remote container carries no mount at all, so nothing on the remote host has
// to exist for it to launch.
func TestRemoteContainerMountsNothing(t *testing.T) {
	cm := newCMTest(t)
	cm.kandevHomeDir = "/kandev-home"
	cm.resolveAgentctlBinary = func() (string, error) {
		t.Fatal("the remote provider resolved a backend-host agentctl path")
		return "", nil
	}
	cm.containerHostFiles = newRemoteContainerHostFiles(SSHRemotePlatform{GOOS: "linux", GOARCH: "arm64"})

	got, err := cm.buildContainerConfig(ContainerConfig{
		AgentConfig: agents.NewCodexACP(),
		InstanceID:  "0123456789abcdef",
		TaskID:      "task-1",
	})
	require.NoError(t, err)

	require.Empty(t, mountPairs(got.Mounts),
		"a remote container must mount nothing; every input is delivered through the daemon")
}

// TestRemoteInputsDeliverHelperAndCredentials covers the launch delivery: the
// helper the container executes and the credentials the agent logs in with,
// both in one archive extracted at the container root.
func TestRemoteInputsDeliverHelperAndCredentials(t *testing.T) {
	agent := seedLocalCredential(t, `{"token":"local"}`)
	writer := &recordingArchiveWriter{}
	inputs := newRemoteInputsForTest(t, writer)

	require.NoError(t, inputs.DeliverLaunchInputs(context.Background(), "cid-1", ContainerConfig{
		AgentConfig: agent,
		InstanceID:  "0123456789abcdef",
	}))

	require.Equal(t, "cid-1", writer.calls[0].containerID)
	require.Equal(t, "/", writer.calls[0].dstPath, "the archive carries its own directories, so it extracts at /")

	entries := writer.entries(t)

	helper, ok := entryByName(entries, "usr/local/bin/agentctl")
	require.True(t, ok, "no helper in %v", entries)
	require.Equal(t, int64(0o755), helper.mode)
	require.Equal(t, "AGENTCTL-ELF", helper.body)

	target := NewCommandBuilder().GetSessionDirTarget(agent)
	require.NotEmpty(t, target, "this agent must declare a session dir target for the test to mean anything")
	wantCred := strings.TrimPrefix(target, "/") + "/.credential-agent/creds.json"
	cred, ok := entryByName(entries, wantCred)
	require.True(t, ok, "credential not delivered at %q; entries = %v", wantCred, entries)
	require.Equal(t, `{"token":"local"}`, cred.body)
	require.Equal(t, int64(credentialFileMode), cred.mode)
}

// TestRemoteInputsNeverNameTheRemoteHome is the regression guard for the
// defect this capability fixes: nothing addressed at the remote user's home
// may appear in what the runtime delivers.
func TestRemoteInputsNeverNameTheRemoteHome(t *testing.T) {
	agent := seedLocalCredential(t, "x")
	writer := &recordingArchiveWriter{}
	inputs := newRemoteInputsForTest(t, writer)

	require.NoError(t, inputs.DeliverLaunchInputs(context.Background(), "cid-1", ContainerConfig{
		AgentConfig: agent,
		InstanceID:  "0123456789abcdef",
	}))

	for _, entry := range writer.entries(t) {
		require.NotContains(t, entry.name, ".kandev/agent-sessions",
			"an entry still addresses the remote host's Kandev home")
		require.NotContains(t, entry.name, ".kandev/bin",
			"an entry still addresses the remote host's Kandev home")
	}
}

// TestRemoteInputsCreateTheSessionDirWithNoCredentials keeps an agent that
// declares a session directory but seeds nothing from starting without it.
func TestRemoteInputsCreateTheSessionDirWithNoCredentials(t *testing.T) {
	writer := &recordingArchiveWriter{}
	inputs := newRemoteInputsForTest(t, writer)

	agent := agents.NewCodexACP()
	require.NoError(t, inputs.DeliverLaunchInputs(context.Background(), "cid-1", ContainerConfig{
		AgentConfig: agent,
		InstanceID:  "0123456789abcdef",
	}))

	target := strings.TrimPrefix(NewCommandBuilder().GetSessionDirTarget(agent), "/") + "/"
	_, ok := entryByName(writer.entries(t), target)
	require.True(t, ok, "session dir %q missing from %v", target, writer.entries(t))
}

// TestRemoteInputsFailWhenTheHelperCannotBeResolved keeps a launch from
// producing a container with no agentctl in it.
func TestRemoteInputsFailWhenTheHelperCannotBeResolved(t *testing.T) {
	writer := &recordingArchiveWriter{}
	inputs := newRemoteInputsForTest(t, writer)
	inputs.resolveAgentctl = func(SSHRemotePlatform) ([]byte, error) {
		return nil, errors.New("no linux/amd64 agentctl in the build tree")
	}

	err := inputs.DeliverLaunchInputs(context.Background(), "cid-1", ContainerConfig{
		AgentConfig: agents.NewCodexACP(),
		InstanceID:  "0123456789abcdef",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "agentctl")
	require.Empty(t, writer.calls, "nothing is delivered when the helper is missing")
}

// TestRemoteInputsTolerateACredentialFailure matches the local Docker path:
// some agents authenticate from the environment or their setup script, so a
// missing credential file must not abort the launch.
func TestRemoteInputsTolerateACredentialFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the declared credential file does not exist
	writer := &recordingArchiveWriter{}
	inputs := newRemoteInputsForTest(t, writer)

	err := inputs.DeliverLaunchInputs(context.Background(), "cid-1", ContainerConfig{
		AgentConfig: newCredentialAgent(".credential-agent/creds.json"),
		InstanceID:  "0123456789abcdef",
	})

	require.NoError(t, err, "a credential failure is a warning, not a launch failure")
	_, ok := entryByName(writer.entries(t), "usr/local/bin/agentctl")
	require.True(t, ok, "the helper is still delivered")
}

// TestRemoteInputsRedeliverOnlyHelpersOnResume keeps a preserved container off
// a stale helper after a backend upgrade, without overwriting the session
// directory the container already holds.
func TestRemoteInputsRedeliverOnlyHelpersOnResume(t *testing.T) {
	agent := seedLocalCredential(t, `{"token":"local"}`)
	writer := &recordingArchiveWriter{}
	inputs := newRemoteInputsForTest(t, writer)

	require.NoError(t, inputs.DeliverHelpers(context.Background(), "cid-9"))

	entries := writer.entries(t)
	_, ok := entryByName(entries, "usr/local/bin/agentctl")
	require.True(t, ok, "the helper must be re-delivered on resume")

	target := strings.TrimPrefix(NewCommandBuilder().GetSessionDirTarget(agent), "/")
	for _, entry := range entries {
		require.False(t, strings.HasPrefix(entry.name, target),
			"resume re-seeded %q into a container that already holds it", entry.name)
	}
}

// TestRemoteInputsSelectTheProbedPlatform fixes the Docker executor's
// unconditional linux/amd64 choice, which is invisible against a local amd64
// daemon and fatal against an arm64 remote.
func TestRemoteInputsSelectTheProbedPlatform(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			writer := &recordingArchiveWriter{}
			inputs := newRemoteContainerInputs(writer, SSHRemotePlatform{GOOS: "linux", GOARCH: arch},
				NewCommandBuilder(), newTestLogger())
			var got SSHRemotePlatform
			inputs.resolveAgentctl = func(p SSHRemotePlatform) ([]byte, error) {
				got = p
				return []byte("ELF"), nil
			}
			inputs.resolveMockAgentBinary = func() (string, error) { return "", nil }

			require.NoError(t, inputs.DeliverHelpers(context.Background(), "cid-1"))
			require.Equal(t, arch, got.GOARCH)
		})
	}
}

// TestRemoteHostFilesRejectAnUnsupportedPlatformBeforeCreate keeps the failure
// ahead of container creation, so an unsupported remote names its own cause
// instead of leaving a container that cannot run its helper.
func TestRemoteHostFilesRejectAnUnsupportedPlatformBeforeCreate(t *testing.T) {
	provider := newRemoteContainerHostFiles(SSHRemotePlatform{GOOS: "plan9", GOARCH: "mips"})
	_, err := provider.AgentctlBinary()
	require.Error(t, err)
}

// TestRemoteInputsDeliverTheMockAgent covers the E2E path. The binary lives in
// the backend's build tree, so the remote daemon cannot mount it.
func TestRemoteInputsDeliverTheMockAgent(t *testing.T) {
	mockPath := filepath.Join(t.TempDir(), "mock-agent")
	require.NoError(t, os.WriteFile(mockPath, []byte("MOCK-ELF"), 0o755))

	writer := &recordingArchiveWriter{}
	inputs := newRemoteInputsForTest(t, writer)
	inputs.resolveMockAgentBinary = func() (string, error) { return mockPath, nil }

	require.NoError(t, inputs.DeliverHelpers(context.Background(), "cid-1"))

	entry, ok := entryByName(writer.entries(t), "usr/local/bin/mock-agent")
	require.True(t, ok, "mock agent missing from %v", writer.entries(t))
	require.Equal(t, "MOCK-ELF", entry.body)
	require.Equal(t, int64(0o755), entry.mode)
}

// TestRemoteReconnectDelegateRedeliversTheHelper is the wiring that keeps a
// resumed container off a stale helper. Without it the delegate restarts the
// container with whatever agentctl it was created with.
func TestRemoteReconnectDelegateRedeliversTheHelper(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	writer := &recordingArchiveWriter{}
	session := &remoteDockerSession{inputs: newRemoteInputsForTest(t, writer)}

	delegate := exec.reconnectDelegate(session)
	require.NotNil(t, delegate.beforeContainerStart,
		"a resumed remote container restarts without its helper being refreshed")

	require.NoError(t, delegate.beforeContainerStart(context.Background(), "cid-7"))
	_, ok := entryByName(writer.entries(t), "usr/local/bin/agentctl")
	require.True(t, ok)
}

// TestLocalReconnectDelegateHasNoRedelivery keeps the local Docker path on the
// daemon calls it already made: its helper is a bind mount, already current.
func TestLocalReconnectDelegateHasNoRedelivery(t *testing.T) {
	require.Nil(t, (&DockerExecutor{logger: newTestLogger()}).beforeContainerStart)
}
