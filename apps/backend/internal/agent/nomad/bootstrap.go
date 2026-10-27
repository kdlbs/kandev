package nomad

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var safeNodeID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// FenceDir is the start fence directory of one environment. A pool environment
// is fenced alone under its node's state root so several environments can share
// a node; a pinned worker keeps the worker-wide fence.
func FenceDir(state EnvironmentState, allocationRoot string) string {
	if state.PlacementMode == ModePool {
		return filepath.Join(state.StateRoot, "environments", state.EnvironmentID, "active")
	}
	return filepath.Join(filepath.Dir(filepath.Dir(allocationRoot)), "worker-state", state.NodeID, "active-environment")
}

// BuildBootstrapScript returns the worker-side bootstrap wrapper. The secret
// environment file is staged mode 0600 over the already pinned SSH channel;
// its values never appear in the Nomad job, task argv, or task logs. The fence
// is deliberately outside Nomad's allocation directory and is never removed
// by this wrapper, so uncertain task exit cannot make the worker look free.
func BuildBootstrapScript(state EnvironmentState, allocationRoot string) (string, error) {
	if err := state.Validate(); err != nil {
		return "", fmt.Errorf("build Nomad bootstrap: invalid state: %w", err)
	}
	root, err := normalizedAbsolutePath(allocationRoot)
	if err != nil || filepath.Clean(root) != state.AllocationRoot {
		return "", fmt.Errorf("build Nomad bootstrap: allocation root differs from immutable state")
	}
	if !safeNodeID.MatchString(state.NodeID) {
		return "", fmt.Errorf("build Nomad bootstrap: NodeID contains unsupported path characters")
	}
	fence := FenceDir(state, root)
	fenceParent := filepath.Dir(fence)
	scratch := ""
	if state.PlacementMode == ModePool {
		scratch = "export TMPDIR=\"${NOMAD_ALLOC_DIR:?missing Nomad allocation directory}/tmp\"\nmkdir -p \"$TMPDIR\"\n"
	}
	owner := strings.Join([]string{state.InstallationID, state.NodeID, state.EnvironmentID, fmt.Sprint(state.Generation), state.OperationID, state.InputDigest, state.JobID}, ":")
	return fmt.Sprintf(`#!/bin/sh
set -eu
: "${KANDEV_AGENTCTL_PATH:?missing agentctl path}"
: "${KANDEV_BOOTSTRAP_ENV_FILE:?missing private bootstrap environment file}"
: "${NOMAD_PORT_agentctl_control:?missing Nomad-reserved control port}"
: "${NOMAD_PORT_agentctl_instance:?missing Nomad-reserved instance port}"
: "${NOMAD_JOB_ID:?missing Nomad job identity}"
: "${NOMAD_ALLOC_ID:?missing Nomad allocation identity}"
: "${KANDEV_NOMAD_NODE_ID:?missing Nomad node identity}"
[ "$NOMAD_JOB_ID" = %s ] || { echo 'Kandev job identity mismatch' >&2; exit 72; }
[ "$KANDEV_NOMAD_NODE_ID" = %s ] || { echo 'Kandev worker identity mismatch' >&2; exit 72; }
agentctl_path=$KANDEV_AGENTCTL_PATH
workspace_root=$KANDEV_WORKSPACE_ROOT
bootstrap_env_file=$KANDEV_BOOTSTRAP_ENV_FILE
node_launch_policy=${KANDEV_NODE_LAUNCH_WRAPPER_FILE:-}
node_allocation=${NOMAD_ALLOC_DIR:-}
control_port=$NOMAD_PORT_agentctl_control
instance_port=$NOMAD_PORT_agentctl_instance
umask 077
mkdir -p "$workspace_root"
mkdir -p %s
if ! mkdir %s 2>/dev/null; then
  echo 'Kandev start fence already exists' >&2
  exit 73
fi
printf '%%s\n' %s "$NOMAD_ALLOC_ID" > %s/owner
chmod 0600 %s/owner
set -a
. "$bootstrap_env_file"
set +a
%sexport KANDEV_AGENTCTL_PATH="$agentctl_path"
export KANDEV_WORKSPACE_ROOT="$workspace_root"
export AGENTCTL_PORT="$control_port"
export AGENTCTL_INSTANCE_PORT_BASE="$instance_port"
export AGENTCTL_INSTANCE_PORT_MAX="$instance_port"
export AGENTCTL_LISTEN_HOST=127.0.0.1
export KANDEV_ACP_IDLE_TIMEOUT=0
if [ -n "$node_launch_policy" ]; then
  export KANDEV_NODE_LAUNCH_WRAPPER_FILE="$node_launch_policy"
  export NOMAD_ALLOC_DIR="${node_allocation:?missing worker allocation directory}"
  exec "$agentctl_path" --require-node-policy --workdir "$workspace_root"
fi
exec "$agentctl_path" --workdir "$workspace_root"
`, shellQuote(state.JobID), shellQuote(state.NodeID), shellQuote(fenceParent), shellQuote(fence), shellQuote(owner), shellQuote(fence), shellQuote(fence), scratch), nil
}

// EncodePrivateEnvironment produces a POSIX environment fragment intended for
// a mode-0600 worker file transferred over the pinned SSH connection. It must
// never be embedded into a Nomad job or command line.
func EncodePrivateEnvironment(values map[string]string) ([]byte, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		if !envKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("private bootstrap environment has invalid key")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(shellQuote(values[key]))
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
