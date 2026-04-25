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

# ---- Repository setup (if configured) ----
{{repository.setup_script}}
`

const defaultWorktreePrepareScript = `#!/bin/bash
# Prepare worktree environment
# Runs after the worktree has already been created/reused by Kandev.
# The script executes with working directory set to {{worktree.path}}.
# Use {{repository.path}} if you need to run commands in the main repository.

# ---- Repository setup (if configured) ----
{{repository.setup_script}}
`

const defaultDockerPrepareScript = `#!/bin/sh
# Prepare Docker container environment (kandev/multi-agent image)
# git, node, and agentctl are already installed in the image

set -eu

# ---- Git identity (optional) ----
{{git.identity_setup}}

# ---- Configure git/gh for HTTPS auth ----
git config --global url."https://github.com/".insteadOf "git@github.com:"
git config --global url."https://github.com/".insteadOf "ssh://git@github.com/"

# Configure GitHub token for gh CLI and git operations
{{github.auth_setup}}

# ---- Clone repository ----
git clone --depth=1 --branch {{repository.branch}} {{repository.clone_url}} {{workspace.path}}
cd {{workspace.path}}

# Strip embedded token from remote URL to avoid persisting credentials in .git/config
git remote set-url origin "$(git remote get-url origin | sed 's|https://[^@]*@github.com/|https://github.com/|')" 2>/dev/null || true

# ---- Repository setup (if configured) ----
{{repository.setup_script}}
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

# ---- Configure git/gh for HTTPS auth (token-based, no SSH keys needed) ----
# Rewrite SSH URLs to HTTPS so git clone git@github.com:... works via token auth
git config --global url."https://github.com/".insteadOf "git@github.com:"
git config --global url."https://github.com/".insteadOf "ssh://git@github.com/"

# Configure GitHub token for gh CLI and git operations
# GH_TOKEN is the primary env var for gh CLI authentication
{{github.auth_setup}}

# ---- Install pnpm globally ----
curl -fsSL https://github.com/pnpm/pnpm/releases/download/v10.32.1/pnpm-linux-x64 -o /usr/local/bin/pnpm
chmod +x /usr/local/bin/pnpm

# ---- Git identity ----
{{git.identity_setup}}

# ---- Clone repository ----
echo "Cloning {{repository.clone_url}} (branch: {{repository.branch}})..."
git clone --depth=1 --quiet --branch {{repository.branch}} {{repository.clone_url}} {{workspace.path}}
cd {{workspace.path}}

# Strip embedded token from remote URL to avoid persisting credentials in .git/config
git remote set-url origin "$(git remote get-url origin | sed 's|https://[^@]*@github.com/|https://github.com/|')" 2>/dev/null || true

# ---- Repository setup (if configured) ----
{{repository.setup_script}}

# ---- Pre-install agent CLI(s) ----
{{kandev.agents.install}}

# ---- Install and start Kandev agent controller ----
echo "Starting agent controller..."
{{kandev.agentctl.install}}
{{kandev.agentctl.start}}
echo "Prepare complete."
`
