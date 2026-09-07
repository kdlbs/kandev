package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
)

const executorTypeSSH = "ssh"

// This hash identifies the built-in Sprites script that shipped before the
// in-place, full-history materialization flow. Upgrade only that exact value;
// user-authored prepare scripts must remain unchanged.
const legacySpritesPrepareScriptSHA256 = "e656f9e51496e1bb1e5cee205058bbeaa7fe1ab37073f600e6c94310ead7be4d"

// DefaultPrepareScript returns the default prepare script for a given executor type string.
func DefaultPrepareScript(executorType string) string {
	switch executorType {
	case "local":
		return defaultLocalPrepareScript
	case "worktree":
		return defaultWorktreePrepareScript
	case "local_docker", "remote_docker":
		return defaultDockerPrepareScript
	case "k8s":
		return defaultKubernetesPrepareScript
	case "sprites":
		return defaultSpritesPrepareScript
	case executorTypeSSH:
		return defaultSSHPrepareScript
	default:
		return ""
	}
}

func isLegacySpritesPrepareScript(script string) bool {
	digest := sha256.Sum256([]byte(script))
	return hex.EncodeToString(digest[:]) == legacySpritesPrepareScriptSHA256
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
#
# SECURITY: the data placeholders below are referenced BARE (no surrounding
# quotes). The scriptengine providers substitute a fully single-quoted,
# self-contained shell token (see scriptengine.shellQuote), so a branch name
# containing shell metacharacters (e.g. "$(...)", backticks, ";") resolves to a
# quoted literal that cannot inject commands. Do NOT wrap these in double
# quotes — double quotes would re-expose $(...) command substitution. Do NOT
# assume they are unquoted data either; the value carries its own quoting.
#
# Keep the complete commit graph so diff and rebase operations have a common
# ancestor.
worktree_branch={{worktree.branch}}
repository_branch={{repository.branch}}
(
  if [ -d {{workspace.path}}/.git ] \
     && [ -n "$worktree_branch" ] \
     && [ "$worktree_branch" != "$repository_branch" ]; then
    cd {{workspace.path}} || exit 0
    if [ "$(git rev-parse --is-shallow-repository 2>/dev/null || echo false)" = true ]; then
      git fetch --unshallow --no-tags origin
    fi
    if git rev-parse --verify "$worktree_branch" >/dev/null 2>&1; then
      git checkout "$worktree_branch"
    elif git fetch --no-tags origin "+refs/heads/${worktree_branch}:refs/remotes/origin/${worktree_branch}" 2>/dev/null; then
      git checkout -b "$worktree_branch" "origin/$worktree_branch"
    else
      git checkout -b "$worktree_branch"
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
# SECURITY: the providers substitute fully single-quoted tokens (shellQuote) for
# repository.branch / repository.clone_url / workspace.path, so a hostile branch
# name or URL cannot break out of the git clone argument even though the
# placeholders are referenced bare here. Do not add double quotes around them.
git clone --depth=1 --branch {{repository.branch}} {{repository.clone_url}} {{workspace.path}}
cd {{workspace.path}}

# Strip embedded token from remote URL to avoid persisting credentials in .git/config
git remote set-url origin "$(git remote get-url origin | sed 's|https://[^@]*@github.com/|https://github.com/|')" 2>/dev/null || true

# ---- Repository setup (if configured) ----
{{repository.setup_script}}
`

const defaultKubernetesPrepareScript = `#!/bin/sh
# Prepare a Kubernetes Pod workspace. The workspace may be a retained PVC
# mounted into a replacement Pod, so repository materialization is idempotent.

set -eu

workspace={{workspace.path}}
repository_url={{repository.clone_url}}
repository_branch={{repository.branch}}
clone_tmp=/opt/kandev/.workspace-clone

# ---- Git identity and HTTPS authentication ----
{{git.identity_setup}}
git config --global --add safe.directory '*'
git config --global url."https://github.com/".insteadOf "git@github.com:"
git config --global url."https://github.com/".insteadOf "ssh://git@github.com/"
{{github.auth_setup}}

mkdir -p "$workspace"
if [ -n "$repository_url" ]; then
  if git -C "$workspace" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    workspace_root=$(git -C "$workspace" rev-parse --show-toplevel)
    if [ "$workspace_root" != "$workspace" ]; then
      echo 'kandev: retained workspace repository root does not match the mount root' >&2
      exit 1
    fi
    workspace_origin=$(git -C "$workspace" remote get-url origin 2>/dev/null || true)
    expected_origin=$(printf '%s\n' "$repository_url" | sed 's|^https://[^/@]*@github.com/|https://github.com/|')
    retained_origin=$(printf '%s\n' "$workspace_origin" | sed 's|^https://[^/@]*@github.com/|https://github.com/|')
    if [ -z "$workspace_origin" ] || [ "$retained_origin" != "$expected_origin" ]; then
      echo 'kandev: retained workspace repository origin does not match the configured repository' >&2
      exit 1
    fi
  else
    # A fresh filesystem-backed PVC may contain only lost+found. Preserve it,
    # clone on the runtime emptyDir, then copy the checkout into the mount root.
    if find "$workspace" -mindepth 1 -maxdepth 1 ! -name lost+found -print -quit | grep -q .; then
      echo 'kandev: retained workspace is non-empty but is not a valid checkout' >&2
      exit 1
    fi
    rm -rf "$clone_tmp"
    trap 'rm -rf "$clone_tmp"' 0 1 2 15
    git clone --depth=1 --branch "$repository_branch" "$repository_url" "$clone_tmp"
    cp -a "$clone_tmp"/. "$workspace"/
    rm -rf "$clone_tmp"
    trap - 0 1 2 15
  fi

  cd "$workspace"

  # Strip embedded token from remote URL to avoid persisting credentials.
  git remote set-url origin "$(git remote get-url origin | sed 's|https://[^@]*@github.com/|https://github.com/|')" 2>/dev/null || true

  # ---- Repository setup (if configured) ----
  {{repository.setup_script}}
else
  cd "$workspace"
fi

# ---- Pre-install agent CLI(s) ----
{{kandev.agents.install}}
`

const defaultSpritesPrepareScript = `#!/bin/bash
# Prepare Sprites.dev cloud sandbox
#
# Pre-installed tools (no need to install):
#   git, curl, wget, gh (GitHub CLI), node, python, go,
#   build-essential, openssh-client, ca-certificates

set -euo pipefail

# SECURITY: the providers substitute a fully single-quoted, self-contained
# shell token (scriptengine.shellQuote) for every DATA placeholder. Each one is
# dereferenced BARE exactly once, into a shell variable, and used as "$var"
# afterwards. Never write "{{...}}" — double quotes re-expose $(...) inside a
# hostile branch name or clone URL, which is the injection the quoting exists
# to prevent.
workspace={{workspace.path}}
repository_url={{repository.clone_url}}
repository_branch={{repository.branch}}

# ---- Configure git/gh for HTTPS auth (token-based, no SSH keys needed) ----
# Rewrite SSH URLs to HTTPS so git clone git@github.com:... works via token auth
git config --global url."https://github.com/".insteadOf "git@github.com:"
git config --global url."https://github.com/".insteadOf "ssh://git@github.com/"

# Kandev can write into the workspace before this script runs, so the checkout
# is not guaranteed to be owned by the user running the prepare script.
git config --global --add safe.directory '*'

# Configure GitHub token for gh CLI and git operations
# GH_TOKEN is the primary env var for gh CLI authentication
{{github.auth_setup}}

# ---- Install pnpm (best effort, never fatal) ----
# Use corepack so the repository packageManager value controls the pnpm version.
# Package-manager setup errors must not stop sandbox preparation.
if ! command -v pnpm >/dev/null 2>&1; then
  corepack enable pnpm >/dev/null 2>&1 || true
fi
if ! pnpm --version >/dev/null 2>&1; then
  npm install -g pnpm >/dev/null 2>&1 || true
fi

# ---- Git identity ----
{{git.identity_setup}}

# ---- Materialize the primary repository ----
# Initialize in place because Office files can exist before preparation starts.
# Fetch a blobless commit graph so later diff and rebase operations retain history.
# The kandev-managed feature-branch checkout is appended as an invariant
# postlude (see KandevBranchCheckoutPostlude) — keep it out of the default
# so old profiles snapshotting this script and the postlude never disagree.
# Never print $repository_url because it can contain a credential.
mkdir -p "$workspace"
if [ -n "$repository_url" ]; then
  scrub_origin() {
    origin_url=$(git -C "$workspace" remote get-url origin 2>/dev/null || true)
    if [ -n "$origin_url" ]; then
      clean_origin_url=$(printf '%s' "$origin_url" | sed 's|https://[^@]*@github.com/|https://github.com/|')
      git -C "$workspace" remote set-url origin "$clean_origin_url" 2>/dev/null || true
    fi
  }
  trap scrub_origin EXIT HUP INT TERM
  if git -C "$workspace" rev-parse --git-dir >/dev/null 2>&1; then
    configured_url=$(git -C "$workspace" config --get remote.origin.url 2>/dev/null || true)
    if [ -z "$configured_url" ]; then
      git -C "$workspace" remote add origin "$repository_url"
    else
      configured_id=$(printf '%s' "$configured_url" | sed 's|://[^/@]*@|://|')
      expected_id=$(printf '%s' "$repository_url" | sed 's|://[^/@]*@|://|')
      if [ "$configured_id" != "$expected_id" ]; then
        echo 'kandev: target origin identity conflict' >&2
        exit 1
      fi
      git -C "$workspace" remote set-url origin "$repository_url"
    fi
  else
    git init -q "$workspace"
    git -C "$workspace" remote add origin "$repository_url"
  fi
  git_dir=$(git -C "$workspace" rev-parse --absolute-git-dir)
  exclude_file="$git_dir/info/exclude"
  mkdir -p "$(dirname "$exclude_file")"
  touch "$exclude_file"
  grep -Fqx '/.kandev/' "$exclude_file" || printf '%s\n' '/.kandev/' >>"$exclude_file"

  if [ -z "$repository_branch" ]; then
    echo 'kandev: target repository has no base branch' >&2
    exit 1
  fi

  git -C "$workspace" config remote.origin.promisor true
  git -C "$workspace" config remote.origin.partialclonefilter blob:none
  echo "Fetching $repository_branch..."
  if ! git -C "$workspace" fetch --filter=blob:none --no-tags origin \
      "+refs/heads/${repository_branch}:refs/remotes/origin/${repository_branch}"; then
    echo 'kandev: target base branch is unavailable' >&2
    exit 1
  fi
  # A reused checkout keeps its current branch, local commits, and untracked
  # files. Only an empty repository needs its initial branch created.
  if ! git -C "$workspace" rev-parse --verify HEAD >/dev/null 2>&1; then
    git -C "$workspace" checkout -q -b "$repository_branch" "refs/remotes/origin/$repository_branch"
  fi

  scrub_origin
  trap - EXIT HUP INT TERM
fi

cd "$workspace"

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

//nolint:dupword // repeated shell `fi` tokens close distinct control-flow blocks.
const defaultSSHPrepareScript = `#!/bin/bash
# Prepare an SSH task workspace on the remote host.
# The script runs before the per-session agentctl instance starts.

set -euo pipefail

workspace={{workspace.path}}
repository_url={{repository.clone_url}}
repository_branch={{repository.branch}}

# ---- Configure git/gh authentication ----
{{github.auth_setup}}

# ---- Git identity ----
{{git.identity_setup}}

mkdir -p "$workspace"

# ---- Materialize or reuse the primary repository ----
# The task directory already contains .kandev session files, so initialize the
# repository in place rather than using git clone into a non-empty directory.
if [ -n "$repository_url" ]; then
  if git -C "$workspace" rev-parse --git-dir >/dev/null 2>&1; then
    configured_url=$(git -C "$workspace" config --get remote.origin.url 2>/dev/null || true)
    if [ -z "$configured_url" ]; then
      git -C "$workspace" remote add origin "$repository_url"
    elif [ "$configured_url" != "$repository_url" ]; then
      echo 'kandev: target origin identity conflict' >&2
      exit 1
    fi
  else
    git init -q "$workspace"
    git -C "$workspace" remote add origin "$repository_url"
  fi

  git_dir=$(git -C "$workspace" rev-parse --absolute-git-dir)
  exclude_file="$git_dir/info/exclude"
  mkdir -p "$(dirname "$exclude_file")"
  touch "$exclude_file"
  grep -Fqx '/.kandev/' "$exclude_file" || printf '%s\n' '/.kandev/' >>"$exclude_file"

  if [ -z "$repository_branch" ]; then
    echo 'kandev: target repository has no base branch' >&2
    exit 1
  fi
  if ! git -C "$workspace" fetch --no-tags origin "+refs/heads/${repository_branch}:refs/remotes/origin/${repository_branch}" >/dev/null 2>&1; then
    echo 'kandev: target base branch is unavailable' >&2
    exit 1
  fi
  # A reused checkout keeps its current branch, local commits, and untracked
  # files. Only an empty repository needs its initial branch created.
  if ! git -C "$workspace" rev-parse --verify HEAD >/dev/null 2>&1; then
    git -C "$workspace" checkout -b "$repository_branch" "origin/$repository_branch"
  fi
fi

cd "$workspace"

# ---- Repository setup (if configured) ----
{{repository.setup_script}}
`
