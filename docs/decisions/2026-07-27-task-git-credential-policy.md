# ADR-2026-07-27-task-git-credential-policy: Separate GitHub Automation From Task Git Credential Policy

> Amended by
> [ADR-2026-08-02-new-workspace-github-access-defaults](2026-08-02-new-workspace-github-access-defaults.md):
> newly created workspaces persist `executor` as their initial task Git policy, while existing
> persisted and legacy fallback behavior remains unchanged.

**Status:** accepted
**Date:** 2026-07-27
**Area:** backend, frontend, security

## Context

A workspace GitHub connection currently selects both the identity used by Kandev background
automation and the credentials injected into task processes. That coupling is surprising for local
users who expect Git to use their host credential manager or SSH setup, and selecting a named
`gh` account does not mean that its host configuration should be inherited by every executor.
Managed task access also depends on a broker-aware `agentctl` and `gh` tool path; a standalone
control process can start successfully while leaving the child Git helper unable to find
`agentctl`.

## Decision

Workspace GitHub automation identity and task Git credential policy are separate workspace-owned
settings:

- `managed` is the backward-compatible fallback for existing missing or invalid settings. Kandev
  injects task/repository-bound broker leases for attached GitHub
  repositories. PAT, named GitHub CLI, GitHub App, and migration-only legacy connections remain
  automation methods rather than task-policy values.
- `executor` is the initial mode persisted for newly created workspaces and remains an explicit
  inheritance mode. Kandev injects no GitHub broker helper or `gh` shim.
  Local and Worktree tasks use credentials visible on the Kandev host; Docker, SSH, Sprites, and
  other remote tasks use credentials configured in that executor.
- An explicit executor-profile `GITHUB_TOKEN` or `GH_TOKEN` remains an unmanaged override. It takes
  precedence when the selected policy is `managed` and is disclosed as an executor-profile token
  whose actor is selected at runtime.

The policy is stored with the non-secret `github_workspace_settings` operational settings, is
copied by workspace-settings copy, and does not create another
`github_workspace_connections.source`. Missing or invalid stored values normalize to `managed` for
backward compatibility.

Managed runtime tools are installed once by the `agentctl` control process and activated only for
an instance carrying a valid broker contract. The managed tool directory contains both the
`agentctl` Git credential helper and the broker-aware `gh` shim. GitHub HTTPS helper configuration
resets the inherited helper chain before adding Kandev's helper, disables terminal prompts, and
fails closed when lease redemption or the helper fails. The contract does not claim to prevent an
arbitrary agent from manually changing a remote to SSH or invoking another credential-bearing
tool; executor isolation and agent permissions remain the boundary for that behavior.

`GIT_CONFIG_COUNT` and its indexed `GIT_CONFIG_KEY_<n>` / `GIT_CONFIG_VALUE_<n>` variables are one
ordered protocol, not independent environment variables. Every task-launch boundary that combines
environment sources must parse, compose, and contiguously reindex valid blocks. Lower-precedence
executor or host entries come first and higher-precedence task entries follow; the longest exact
suffix/prefix overlap is emitted once when a block has already been forwarded through a control
process. Kandev may reset the GitHub credential-helper chain with a later empty helper entry, but it
must preserve unrelated entries such as `core.hooksPath`, `safe.directory`, URL rewrites, and notes
configuration. Malformed indexed blocks fail task environment preparation with a credential-safe
diagnostic instead of being partially overlaid. Repository-clone subprocesses remain an explicit
exception: they intentionally build an isolated Git environment and strip ambient indexed config.

Initial launch and resume use the same policy resolver. After each successful launch or resume,
Kandev stores a versioned, non-secret `git_credential_snapshot` in task-session metadata. It
records the selected policy, effective source (`workspace`, `executor_profile`, or `executor`),
workspace method when known, known actor label or `runtime_selected`, transport, executor type,
and capture time. It never stores a token, lease, helper path, or credential-file path. The Changes
panel reads this launch-time snapshot; it does not probe the current host or executor and does not
rewrite history when workspace settings later change.

The branch disclosure shows the snapshot alongside branch/base-branch information. Fine-pointer
desktop users receive hover and keyboard-focus access; coarse-pointer users receive the same
content in a tappable drawer. GitHub settings explain every automation method in visible text and
provide a supplementary information disclosure describing how each method reaches tasks under
both policies.

Named GitHub CLI credentials remain deterministic host/login selections. Their derived bearer
tokens are cached in memory for no more than five minutes before Kandev asks the host CLI for that
exact account again; cache invalidation on connection generation changes remains immediate.

## Consequences

- Users can keep Kandev background automation on a PAT, named CLI account, or App while choosing
  traditional executor Git behavior for tasks.
- Selecting GitHub CLI authentication no longer implies host credential inheritance. Managed mode
  still resolves the named account on the Kandev host and brokers its bearer token to tasks.
- Existing workspaces remain managed without a migration-time behavior change.
- Host and executor Git tooling expressed through indexed environment config survives Kandev
  credential injection; Kandev's later GitHub helper entries retain their intended precedence.
- The UI can truthfully identify managed actors while labeling inherited and profile-token actors
  as runtime-selected instead of guessing.
- Resume may pick up a changed workspace policy or automation connection, and its new snapshot
  replaces the prior launch snapshot only after the resume succeeds.
- Managed HTTPS Git and `gh` fail closed, but a local/Worktree agent still runs with host process
  authority; this setting is credential routing, not a sandbox.
- Environment composition becomes fallible when an indexed Git config block is malformed, so
  launch errors can identify the invalid source instead of silently producing a different Git
  configuration.

## Alternatives Considered

### Treat executor inheritance as a fourth GitHub connection source

Rejected. It would conflate Kandev's background automation identity with task transport again and
would leave watches and repository discovery without a defined credential.

### Infer host inheritance from the named GitHub CLI method

Rejected. The named CLI selection is a deterministic automation identity. Remote executors cannot
inherit the Kandev host's CLI configuration, and local inference would make the same setting mean
different security policies by executor.

### Display the current workspace connection instead of a session snapshot

Rejected. Settings can change after a task starts, so current status can misrepresent the
credential contract under which the visible session actually launched.

### Probe inherited credentials to discover the actor

Rejected. Git credential managers and SSH agents may be interactive, may choose by URL at command
time, and should not be invoked merely to render UI. Unknown inherited actors are labeled
`runtime_selected`.
