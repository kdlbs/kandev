package lifecycle

// DefaultPrepareScript returns the default prepare script for a given executor type string.
func DefaultPrepareScript(executorType string) string {
	switch executorType {
	case "local":
		return defaultLocalPrepareScript
	case "worktree":
		return defaultWorktreePrepareScript
	case "local_docker", "remote_docker":
		return defaultDockerPrepareScript
	case "sprites":
		return defaultSpritesPrepareScript
	default:
		return ""
	}
}

const defaultLocalPrepareScript = `#!/bin/bash
# Prepare local environment
# Runs before launching the local agent runtime.
# The script executes with working directory set to {{workspace.path}}.
# Use {{repository.path}} when you need the canonical repository root path.
`

const defaultWorktreePrepareScript = `#!/bin/bash
# Prepare worktree environment
# Runs after the worktree has already been created/reused by Kandev.
# The script executes with working directory set to {{worktree.path}}.
# Use {{repository.path}} if you need to run commands in the main repository.
`

const defaultDockerPrepareScript = `#!/bin/bash
# Prepare Docker container environment
# This runs inside the Docker container after it starts

set -euo pipefail

# ---- System dependencies ----
apt-get update -qq
apt-get install -y -qq git curl ca-certificates > /dev/null 2>&1

# ---- Node.js (required for agentctl) ----
if ! command -v node &> /dev/null; then
  curl -fsSL https://deb.nodesource.com/setup_22.x | bash - > /dev/null 2>&1
  apt-get install -y -qq nodejs > /dev/null 2>&1
fi

# ---- Git identity (optional) ----
{{git.identity_setup}}

# ---- Clone repository ----
git clone --depth=1 --branch {{repository.branch}} {{repository.clone_url}} {{workspace.path}}
cd {{workspace.path}}

# ---- Repository setup (if configured) ----
{{repository.setup_script}}

# ---- Install and start Kandev agent controller ----
{{kandev.agentctl.install}}
{{kandev.agentctl.start}}
`

const defaultSpritesPrepareScript = `#!/bin/bash
# Prepare Sprites.dev cloud sandbox
#
# Pre-installed tools (no need to install):
#   git, curl, wget, gh (GitHub CLI), node, python, go,
#   build-essential, openssh-client, ca-certificates

set -euo pipefail

# ---- Add SSH host keys (prevent "Host key verification failed") ----
mkdir -p ~/.ssh
ssh-keyscan -t ed25519 github.com gitlab.com bitbucket.org >> ~/.ssh/known_hosts 2>/dev/null

# ---- Git identity ----
{{git.identity_setup}}

# ---- Clone repository ----
echo "Cloning {{repository.clone_url}} (branch: {{repository.branch}})..."
git clone --depth=1 --quiet --branch {{repository.branch}} {{repository.clone_url}} {{workspace.path}}
cd {{workspace.path}}

# ---- Repository setup (if configured) ----
{{repository.setup_script}}

# ---- Install and start Kandev agent controller ----
echo "Starting agent controller..."
{{kandev.agentctl.install}}
{{kandev.agentctl.start}}
echo "Prepare complete."
`
