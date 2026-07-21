package lifecycle

import (
	"os/exec"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentruntime"
)

// MetadataKeyNativeBinary, when present in a launch's metadata, names the
// agent's standalone CLI binary that was found in the execution environment.
// It is set per launch by the remote binary probe (SSH preflight) and read by
// preferNativeBinary so the command builder emits the native binary instead of
// `npx -y <pkg>`. Not persisted across resumes — the probe re-runs each launch
// so a binary installed (or removed) between runs is always reflected.
const MetadataKeyNativeBinary = "native_binary"

// preferNativeBinary reports whether agentConfig's standalone CLI should be
// used instead of `npx -y <pkg>` for this launch. It honours the same
// "is the binary actually present in the execution environment?" check the SSH
// preflight already performs, applied per runtime:
//
//   - standalone (local_pc / worktree): the agent subprocess runs directly on
//     this host, so the backend's PATH is the execution environment — probe it
//     with exec.LookPath.
//   - ssh: the remote preflight probed the binary and recorded a hit in meta.
//   - docker / sprites / remote_docker: a controlled image that pulls npx from
//     the public registry — keep npx.
func (m *Manager) preferNativeBinary(agentConfig agents.Agent, runtime agentruntime.Runtime, meta map[string]interface{}) bool {
	nb, ok := agentConfig.(agents.NativeBinaryAgent)
	if !ok {
		return false
	}
	name := nb.NativeBinaryName()
	if name == "" {
		return false
	}
	switch runtime {
	case agentruntime.RuntimeStandalone:
		_, err := exec.LookPath(name)
		return err == nil
	case agentruntime.RuntimeSSH:
		return getMetadataString(meta, MetadataKeyNativeBinary) == name
	default:
		return false
	}
}
