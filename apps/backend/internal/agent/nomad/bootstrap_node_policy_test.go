//go:build unix

package nomad

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBootstrapPreservesNodePolicyAgainstPrivateEnvironment(t *testing.T) {
	dir := t.TempDir()
	state := validTestState()
	state.WorkspaceRoot = filepath.Join(dir, "workspaces")
	state.WorkspacePath = filepath.Join(state.WorkspaceRoot, "environments", state.EnvironmentID, "g3")
	state.AllocationRoot = filepath.Join(dir, "allocations")
	script, err := BuildBootstrapScript(state, state.AllocationRoot)
	require.NoError(t, err)
	environment := filepath.Join(dir, "private.env")
	require.NoError(t, os.WriteFile(environment, []byte("KANDEV_NODE_LAUNCH_WRAPPER_FILE=/untrusted\nNOMAD_ALLOC_DIR=/untrusted\n"), 0600))
	fake := filepath.Join(dir, "agentctl")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" \"$KANDEV_NODE_LAUNCH_WRAPPER_FILE\" \"$NOMAD_ALLOC_DIR\"\n"), 0700))
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "KANDEV_AGENTCTL_PATH=" + fake, "KANDEV_BOOTSTRAP_ENV_FILE=" + environment,
		"KANDEV_WORKSPACE_ROOT=" + state.WorkspacePath, "NOMAD_PORT_agentctl_control=33001", "NOMAD_PORT_agentctl_instance=33002",
		"NOMAD_JOB_ID=" + state.JobID, "NOMAD_ALLOC_ID=allocation-id", "KANDEV_NOMAD_NODE_ID=" + state.NodeID,
		"KANDEV_NODE_LAUNCH_WRAPPER_FILE=/trusted/node.json", "NOMAD_ALLOC_DIR=" + dir}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	require.Equal(t, "--require-node-policy", lines[0])
	require.Equal(t, []string{"/trusted/node.json", dir}, lines[len(lines)-2:])
}
