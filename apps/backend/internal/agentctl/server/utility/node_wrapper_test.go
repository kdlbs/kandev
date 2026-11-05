package utility

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/launchwrapper"
)

type nodeUtilityFixture struct {
	dir, node, profile, receipt string
}

// newNodeUtilityFixture installs recording wrappers plus agent stubs named like
// allow-listed commands. Every wrapper records its own path and arguments, then
// execs the rest of its command line.
func newNodeUtilityFixture(t *testing.T) nodeUtilityFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	f := nodeUtilityFixture{dir: t.TempDir()}
	f.node = filepath.Join(f.dir, "node-wrapper")
	f.profile = filepath.Join(f.dir, "profile-wrapper")
	f.receipt = filepath.Join(f.dir, "receipt")
	record := "#!/bin/sh\nprintf '%s\\n' \"$0\" \"$@\" >> \"$RECEIPT\"\nshift\nexec \"$@\"\n"
	writeNodeStub(t, f.node, record)
	writeNodeStub(t, f.profile, record)
	for _, name := range []string{"mock-agent", "codex"} {
		writeNodeStub(t, filepath.Join(f.dir, name), "#!/bin/sh\nexit 1\n")
	}
	writeNodeStub(t, filepath.Join(f.dir, "opencode"), "#!/bin/sh\necho opencode/model\n")
	t.Setenv("PATH", f.dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return f
}

func (f nodeUtilityFixture) nodeWrapper() launchwrapper.Node {
	return launchwrapper.Node{
		Spec:     launchwrapper.Spec{Version: launchwrapper.Version, Source: launchwrapper.SourceNode, Argv: []string{f.node, "--"}},
		Required: true,
	}
}

func (f nodeUtilityFixture) cfg(command ...string) *InferenceConfigDTO {
	return &InferenceConfigDTO{Command: command, WorkDir: f.dir, Env: map[string]string{"RECEIPT": f.receipt}}
}

func (f nodeUtilityFixture) lines(t *testing.T) []string {
	t.Helper()
	got, err := os.ReadFile(f.receipt)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(got)), "\n")
}

// @covers AC-AGENTS-NODEWRAP-002.1 AC-AGENTS-NODEWRAP-003.1
func TestNodeWrapperAppliesToACPProbeOutermost(t *testing.T) {
	f := newNodeUtilityFixture(t)
	executor := NewACPInferenceExecutor(zap.NewNop())
	executor.SetNodeLaunchWrapper(f.nodeWrapper())
	cfg := f.cfg("mock-agent")
	cfg.LaunchWrapper = launchwrapper.Profile([]string{f.profile, "--"})
	resp, err := executor.Probe(context.Background(), &ProbeRequest{AgentID: "mock-agent", InferenceConfig: cfg})
	require.NoError(t, err)
	require.False(t, resp.Success)
	lines := f.lines(t)
	require.GreaterOrEqual(t, len(lines), 5, lines)
	require.Equal(t, f.node, lines[0], "node wrapper runs first")
	require.Equal(t, []string{"--", f.profile, "--"}, lines[1:4], "profile wrapper runs inside node")
	require.Equal(t, filepath.Join(f.dir, "mock-agent"), lines[4])
	require.NotContains(t, resp.Error, f.node)
}

// @covers AC-AGENTS-NODEWRAP-003.1
func TestNodeWrapperAppliesToACPExecute(t *testing.T) {
	f := newNodeUtilityFixture(t)
	executor := NewACPInferenceExecutor(zap.NewNop())
	executor.SetNodeLaunchWrapper(f.nodeWrapper())
	resp, err := executor.Execute(context.Background(), &PromptRequest{AgentID: "mock-agent", Prompt: "hi", InferenceConfig: f.cfg("mock-agent")})
	require.NoError(t, err)
	require.False(t, resp.Success)
	lines := f.lines(t)
	require.NotEmpty(t, lines)
	require.Equal(t, f.node, lines[0])
	require.Equal(t, filepath.Join(f.dir, "mock-agent"), lines[2])
	require.NotContains(t, resp.Error, f.node)
}

// @covers AC-AGENTS-NODEWRAP-003.1
func TestNodeWrapperAppliesToOpenCodeModelListing(t *testing.T) {
	f := newNodeUtilityFixture(t)
	cfg := f.cfg("opencode")
	cfg.node = f.nodeWrapper()
	models, err := probeOpenCodeModels(context.Background(), filepath.Join(f.dir, "opencode"), f.dir, false, cfg)
	require.NoError(t, err)
	require.Len(t, models, 1)
	lines := f.lines(t)
	require.Equal(t, []string{f.node, "--", filepath.Join(f.dir, "opencode"), "models"}, lines)
}

// @covers AC-AGENTS-NODEWRAP-003.1
func TestNodeWrapperAppliesToCodexAppServer(t *testing.T) {
	f := newNodeUtilityFixture(t)
	executor := NewCodexAppServerInferenceExecutor(zap.NewNop())
	executor.SetNodeLaunchWrapper(f.nodeWrapper())
	cfg := f.cfg("codex", "app-server")
	resp, err := executor.Probe(context.Background(), &ProbeRequest{AgentID: "codex", InferenceConfig: cfg})
	require.NoError(t, err)
	require.False(t, resp.Success)
	lines := f.lines(t)
	require.NotEmpty(t, lines)
	require.Equal(t, f.node, lines[0])
	require.Equal(t, filepath.Join(f.dir, "codex"), lines[2])

	prompt, err := executor.Execute(context.Background(), &PromptRequest{AgentID: "codex", Prompt: "hi", InferenceConfig: f.cfg("codex", "app-server")})
	require.NoError(t, err)
	require.False(t, prompt.Success)
	require.Equal(t, f.node, f.lines(t)[len(lines)])
}

// @covers AC-AGENTS-NODEWRAP-003.3
func TestNodeWrapperRequiredButMissingRefusesEverySeam(t *testing.T) {
	f := newNodeUtilityFixture(t)
	missing := launchwrapper.Node{Required: true}
	acp := NewACPInferenceExecutor(zap.NewNop())
	acp.SetNodeLaunchWrapper(missing)
	codex := NewCodexAppServerInferenceExecutor(zap.NewNop())
	codex.SetNodeLaunchWrapper(missing)

	probe, err := acp.Probe(context.Background(), &ProbeRequest{AgentID: "mock-agent", InferenceConfig: f.cfg("mock-agent")})
	require.NoError(t, err)
	require.False(t, probe.Success)
	prompt, err := acp.Execute(context.Background(), &PromptRequest{AgentID: "mock-agent", Prompt: "hi", InferenceConfig: f.cfg("mock-agent")})
	require.NoError(t, err)
	require.False(t, prompt.Success)
	cfg := f.cfg("opencode")
	cfg.node = missing
	_, err = probeOpenCodeModels(context.Background(), filepath.Join(f.dir, "opencode"), f.dir, false, cfg)
	require.ErrorIs(t, err, launchwrapper.ErrNodeRequired)
	cprobe, err := codex.Probe(context.Background(), &ProbeRequest{AgentID: "codex", InferenceConfig: f.cfg("codex", "app-server")})
	require.NoError(t, err)
	require.False(t, cprobe.Success)
	cprompt, err := codex.Execute(context.Background(), &PromptRequest{AgentID: "codex", Prompt: "hi", InferenceConfig: f.cfg("codex", "app-server")})
	require.NoError(t, err)
	require.False(t, cprompt.Success)
	require.Empty(t, f.lines(t), "no child, wrapped or not, was started")
}

// @covers AC-AGENTS-NODEWRAP-003.4
func TestNoNodeWrapperLeavesACPLaunchUnchanged(t *testing.T) {
	f := newNodeUtilityFixture(t)
	cfg := f.cfg("mock-agent")
	argv, err := prepareACPLaunch(cfg, "mock-agent", []string{"mock-agent", "--acp"}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"mock-agent", "--acp"}, argv)
}

// @covers AC-AGENTS-NODEWRAP-003.2
func TestNodeWrapperSuppressesRuntimeObservationExecutables(t *testing.T) {
	f := newNodeUtilityFixture(t)
	cfg := f.cfg("mock-agent")
	require.Empty(t, observationPrefix(cfg))
	cfg.node = f.nodeWrapper()
	require.NotEmpty(t, observationPrefix(cfg), "a node wrapper counts as a prefix, so observation commands do not run unwrapped")
}

// @covers AC-AGENTS-NODEWRAP-001.1
func TestClientCannotSetNodeWrapper(t *testing.T) {
	var cfg InferenceConfigDTO
	require.NoError(t, json.Unmarshal([]byte(`{"command":["mock-agent"],"node":{"Spec":{"argv":["/bin/evil"]}},"node_launch_wrapper":{"argv":["/bin/evil"]}}`), &cfg))
	require.False(t, cfg.node.Active())
}

func writeNodeStub(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
}
