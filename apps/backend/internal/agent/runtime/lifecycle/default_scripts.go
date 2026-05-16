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

// KandevBranchCheckoutPostlude returns a kandev-managed shell snippet that
// guarantees the session's feature branch is checked out inside the
// workspace, no matter what the user's stored prepare_script does.
//
// Why a postlude instead of just relying on the default script: profiles
// created in the UI snapshot the *then-current* default into their stored
// prepare_script field. When kandev's default is updated to add a new
// kandev-managed step (like the worktree-branch checkout), older profiles
// silently miss it forever. Making the checkout an invariant — appended
// after the user's script — keeps the contract regardless of which default
// the user happens to have stored.
//
// The snippet is wrapped in a subshell + `|| true` so any failure (e.g. the
// user's script never produced /workspace, or the branch is the same as the
// base) is benign and doesn't block agentctl from starting.
//
//nolint:dupword // two `fi` tokens close two distinct shell blocks.
func KandevBranchCheckoutPostlude() string {
	return `

# ---- kandev-managed: ensure session feature branch is checked out ----
# Appended automatically after the user's prepare script. Idempotent and
# non-destructive: prefer an existing local branch (which may carry unpushed
# work after a container resume), then fall through to a fresh tracking
# branch off origin, and only as a last resort create the branch off HEAD.
# The previous "git checkout -B feature origin/feature" form was destructive
# for the resume case — overwriting local commits with the remote tip.
(
  if [ -d "{{workspace.path}}/.git" ] \
     && [ -n "{{worktree.branch}}" ] \
     && [ "{{worktree.branch}}" != "{{repository.branch}}" ]; then
    cd "{{workspace.path}}" || exit 0
    if git rev-parse --verify "{{worktree.branch}}" >/dev/null 2>&1; then
      git checkout "{{worktree.branch}}"
    elif git fetch --depth=1 origin "{{worktree.branch}}" 2>/dev/null; then
      git checkout -b "{{worktree.branch}}" "origin/{{worktree.branch}}"
    else
      git checkout -b "{{worktree.branch}}"
    fi
  fi
) || true
`
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

# Mounted local remotes and workspaces can be owned by a host UID that does
# not match the container user.
git config --global --add safe.directory '*'

# ---- Configure git/gh for HTTPS auth ----
git config --global url."https://github.com/".insteadOf "git@github.com:"
git config --global url."https://github.com/".insteadOf "ssh://git@github.com/"

# Configure GitHub token for gh CLI and git operations
{{github.auth_setup}}

# ---- Clone repository ----
# The kandev-managed feature-branch checkout is appended as an invariant
# postlude (see KandevBranchCheckoutPostlude) — keep it out of the default
# so old profiles snapshotting this script and the postlude never disagree.
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
# The kandev-managed feature-branch checkout is appended as an invariant
# postlude (see KandevBranchCheckoutPostlude) — keep it out of the default
# so old profiles snapshotting this script and the postlude never disagree.
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
