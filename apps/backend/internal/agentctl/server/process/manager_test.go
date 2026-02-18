package process

import (
	"context"
	"io"
	"os/exec"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAdapter is a minimal AgentAdapter for testing the startup sequence.
type stubAdapter struct {
	connectCalled bool
	updatesCh     chan adapter.AgentEvent
}

func newStubAdapter() *stubAdapter {
	return &stubAdapter{updatesCh: make(chan adapter.AgentEvent, 10)}
}

func (s *stubAdapter) PrepareEnvironment() (map[string]string, error) { return nil, nil }
func (s *stubAdapter) PrepareCommandArgs() []string                   { return nil }
func (s *stubAdapter) Connect(stdin io.Writer, stdout io.Reader) error {
	s.connectCalled = true
	return nil
}
func (s *stubAdapter) Initialize(context.Context) error              { return nil }
func (s *stubAdapter) GetAgentInfo() *adapter.AgentInfo              { return nil }
func (s *stubAdapter) NewSession(_ context.Context, _ []types.McpServer) (string, error) {
	return "", nil
}
func (s *stubAdapter) LoadSession(context.Context, string) error                    { return nil }
func (s *stubAdapter) Prompt(context.Context, string, []v1.MessageAttachment) error { return nil }
func (s *stubAdapter) Cancel(context.Context) error                                 { return nil }
func (s *stubAdapter) Updates() <-chan adapter.AgentEvent                            { return s.updatesCh }
func (s *stubAdapter) GetSessionID() string                                         { return "" }
func (s *stubAdapter) GetOperationID() string                                       { return "" }
func (s *stubAdapter) SetPermissionHandler(adapter.PermissionHandler)               {}
func (s *stubAdapter) Close() error {
	close(s.updatesCh)
	return nil
}
func (s *stubAdapter) RequiresProcessKill() bool { return false }

func TestStartProcessPipes_CreatesAllPipes(t *testing.T) {
	m := &Manager{
		cmd: exec.Command("cat"),
	}

	err := m.startProcessPipes()
	require.NoError(t, err)
	assert.NotNil(t, m.stdin, "stdin pipe should be created")
	assert.NotNil(t, m.stdout, "stdout pipe should be created")
	assert.NotNil(t, m.stderr, "stderr pipe should be created")

	// Clean up
	_ = m.stdin.Close()
}

func TestStartProcessPipes_FailsAfterProcessStarted(t *testing.T) {
	// Regression test: documents the bug where cmd.Start() was called inside
	// buildFinalCommand() before pipes were created.
	// exec.Cmd pipes cannot be created after Start() is called.
	cmd := exec.Command("sleep", "10")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	_, err := cmd.StdinPipe()
	assert.Error(t, err, "pipes cannot be created after process starts")
}

func TestBuildFinalCommand_DoesNotStartProcess(t *testing.T) {
	log := newTestLogger(t)
	stub := newStubAdapter()

	m := &Manager{
		cfg: &config.InstanceConfig{
			AgentArgs: []string{"cat"},
			WorkDir:   t.TempDir(),
		},
		logger:  log,
		adapter: stub,
	}

	err := m.buildFinalCommand()
	require.NoError(t, err)
	assert.NotNil(t, m.cmd, "cmd should be created")
	assert.Nil(t, m.cmd.Process, "process should not be started yet")
}

func TestManager_Start_PipesCreatedBeforeProcessStart(t *testing.T) {
	log := newTestLogger(t)
	stub := newStubAdapter()
	workDir := t.TempDir()

	m := &Manager{
		cfg: &config.InstanceConfig{
			AgentArgs: []string{"cat"},
			WorkDir:   workDir,
			AgentEnv:  []string{"PATH=/usr/bin:/bin"},
		},
		logger:             log,
		adapter:            stub,
		adapterCfg:         &adapter.Config{WorkDir: workDir},
		updatesCh:          make(chan adapter.AgentEvent, 100),
		pendingPermissions: make(map[string]*PendingPermission),
		workspaceTracker:   NewWorkspaceTracker(workDir, log),
	}
	m.status.Store(StatusStopped)
	m.exitCode.Store(-1)

	// Skip buildAdapterConfig since we already have a stub adapter.
	// Call the sub-steps in the same order as Start() to verify correctness.
	err := m.buildFinalCommand()
	require.NoError(t, err)
	assert.Nil(t, m.cmd.Process, "process must not be started after buildFinalCommand")

	err = m.startProcessPipes()
	require.NoError(t, err)
	assert.NotNil(t, m.stdin)
	assert.NotNil(t, m.stdout)
	assert.NotNil(t, m.stderr)

	// Now start the process — this is the fix: Start() happens after pipes.
	err = m.cmd.Start()
	require.NoError(t, err)
	assert.NotNil(t, m.cmd.Process, "process should be running")

	// Adapter connects after process starts.
	err = stub.Connect(m.stdin, m.stdout)
	require.NoError(t, err)
	assert.True(t, stub.connectCalled)

	// Clean up: close stdin so cat exits, then wait.
	_ = m.stdin.Close()
	_ = m.cmd.Wait()
}
