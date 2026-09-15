#!/bin/sh
# Prepare a Kubernetes Pod workspace. The workspace may be a retained PVC
# mounted into a replacement Pod, so repository materialization is idempotent.

set -eu

# Bound each client call as well as the overall readiness window.
ready_timeout=${DOCKER_READY_TIMEOUT_SECONDS:-60}
case "$ready_timeout" in ''|*[!0-9]*) echo 'Invalid Docker readiness timeout' >&2; exit 1;; esac
[ "$ready_timeout" -ge 1 ] && [ "$ready_timeout" -le 120 ]
deadline=$(( $(date +%s) + ready_timeout ))
until timeout 3 docker info >/dev/null 2>&1; do
  if [ "$(date +%s)" -ge "$deadline" ]; then
    echo 'Docker daemon did not become ready before preparation deadline' >&2
    exit 1
  fi
  sleep 1
done

create_workspace_caches() {
  mkdir -p "$workspace/.cache/npm" "$workspace/.cache/pip" "$workspace/.cache/go-build" \
    "$workspace/.cache/go-mod" "$workspace/.npm-global" "$workspace/.pnpm" \
    "$workspace/.pnpm-store" "$workspace/.cache/cargo"
}

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
    # The PVC mount root can be group-writable without being owned by the
    # container user. Copy content without restoring source ownership or times.
    cp -R "$clone_tmp"/. "$workspace"/
    rm -rf "$clone_tmp"
    trap - 0 1 2 15
  fi

  cd "$workspace"

  # Strip embedded token from remote URL to avoid persisting credentials.
  git remote set-url origin "$(git remote get-url origin | sed 's|https://[^@]*@github.com/|https://github.com/|')" 2>/dev/null || true

  create_workspace_caches

  # ---- Repository setup (if configured) ----
  {{repository.setup_script}}
else
  cd "$workspace"
  create_workspace_caches
fi

# ---- Pre-install agent CLI(s) ----
{{kandev.agents.install}}
