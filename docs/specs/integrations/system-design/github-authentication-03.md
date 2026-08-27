---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
created: 2026-07-19
owners:
  - Kandev
---
# Workspace GitHub Authentication System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Scenarios

- **GIVEN** a brand-new Kandev database and an authenticated active host `gh` account, **WHEN** the
  initial workspace is seeded, **THEN** its automation connection records that exact CLI
  host/login, its task access is executor inheritance, and its first task does not request a
  managed credential lease.
- **GIVEN** an administrator or internal trusted caller and an authenticated active host `gh`
  account, **WHEN** a workspace is created, **THEN** the workspace records that exact named CLI
  automation connection and executor task access.
- **GIVEN** a non-admin member and an authenticated server-operator `gh` account, **WHEN** the
  member creates a workspace, **THEN** the workspace has executor task access but no automatic
  GitHub automation connection.
- **GIVEN** no usable authenticated host `gh` account, **WHEN** an authorized caller creates a
  workspace, **THEN** workspace creation succeeds, task access is executor inheritance, and
  GitHub automation remains disconnected.
- **GIVEN** an existing installation with managed, disconnected, or legacy missing workspace
  settings, **WHEN** Kandev upgrades, **THEN** those connections and task policies remain
  unchanged.

- **GIVEN** a workspace GitHub status is visible, **WHEN** the user refreshes it and the status
  request is still pending, **THEN** the existing workspace content remains visible and usable
  while the refresh control is busy, on both desktop and mobile.
- **GIVEN** the user navigates to a workspace with no loaded GitHub status, **WHEN** its initial
  status request is pending, **THEN** the UI shows the connection-status placeholder and no status
  from the previous workspace.
- **GIVEN** two workspaces, **WHEN** each selects a different App registration and installation,
  **THEN** status, tokens, webhooks, repositories, actors, and revocation remain isolated.
- **GIVEN** two workspaces intentionally reuse one registration, **WHEN** each installs it into a
  different account, **THEN** each workspace receives only its own installation and repository
  scope while the UI identifies the shared root App identity.
- **GIVEN** a private managed App, **WHEN** creation completes, **THEN** the UI says it is installable
  only on its owner and does not imply Marketplace publication.
- **GIVEN** a user chooses public, **WHEN** the manifest is submitted, **THEN** `public: true` is sent
  and the confirmation explains that installation approval still controls repository access.
- **GIVEN** a correctly configured existing App, **WHEN** the import is verified, **THEN** it appears
  in the workspace chooser without becoming the active connection until installation succeeds.
- **GIVEN** an imported App misses a required GitHub setting, **WHEN** validation or installation
  runs, **THEN** the guide identifies the exact setting without returning submitted secrets.
- **GIVEN** App creation, import, or installation is canceled, **WHEN** the user returns, **THEN** the
  previous workspace automation connection remains active.
- **GIVEN** an App workspace with personal OAuth, **WHEN** it switches registration or to PAT/CLI,
  **THEN** the incompatible personal connection is removed and its old tokens cannot be resolved.
- **GIVEN** a webhook for registration A, **WHEN** it is sent to registration B's route or signature,
  **THEN** no delivery or workspace state is mutated.
- **GIVEN** a PAT or named CLI workspace, **WHEN** an agent uses the managed credential helper,
  **THEN** it receives that workspace's automation token and the UI does not promise provider-side
  repository narrowing.
- **GIVEN** a named CLI workspace in managed mode, **WHEN** a task launches, **THEN** Kandev resolves
  the selected host/login, makes both managed `git` and `gh` available in standalone and remote
  runtimes, and does not depend on the host's currently active CLI account.
- **GIVEN** an authorized user opens a terminal, uses a passthrough-agent PTY, or starts a
  task-scoped command in a managed task, **WHEN** it accesses an attached GitHub repository,
  **THEN** it receives the same task-scoped broker helper and `gh` shim environment as the agent
  subprocess.
- **GIVEN** an unauthorised user or a terminal for another task environment, **WHEN** it attempts to
  open a terminal or start a process, **THEN** it cannot receive the managed task Git environment.
- **GIVEN** a workspace selects executor inheritance, **WHEN** a Local/Worktree or remote task
  launches, **THEN** Kandev injects no broker helper/shim and the task uses host-visible or
  executor-configured credentials respectively.
- **GIVEN** a configured workspace, **WHEN** the user views Workspace GitHub access, **THEN** one
  compact summary identifies both the workspace automation identity and the effective task access
  mode without rendering a separate Task Git credentials settings section.
- **GIVEN** a PAT or named CLI workspace, **WHEN** the user views Workspace GitHub access, **THEN**
  the page states that the same account powers My GitHub and user-triggered actions without
  rendering a separate My GitHub identity section, and accessible help explains both the workspace
  identity and task-access summary.
- **GIVEN** a workspace status with GitHub rate-limit snapshots, **WHEN** the user hovers, focuses,
  or taps **Show GitHub API limits**, **THEN** the disclosure shows the remaining and total API,
  GraphQL query, and Search quotas with reset timing for that workspace connection.
- **GIVEN** a PAT or named CLI connection draft and a changed task access mode, **WHEN** the user
  presses the dialog's single **Save changes** action, **THEN** both drafts are persisted, the dialog
  closes only after both succeed, and reopening shows the selected account and task mode.
- **GIVEN** a user enters an invalid replacement PAT, **WHEN** GitHub rejects it during **Save
  changes**, **THEN** the dialog remains open, the submitted PAT remains available for correction,
  an error is shown, and the previously active workspace connection is unchanged.
- **GIVEN** a configured PAT has expired or been revoked, **WHEN** the user opens My GitHub and the
  provider data request returns 401, **THEN** the page remains on `/github`, shows an authentication
  loading error, and does not navigate to the Kandev login screen.
- **GIVEN** a changed task access mode but no PAT or CLI connection change, **WHEN** the user presses
  **Save changes**, **THEN** only the task policy is persisted and the selected automation identity
  remains unchanged.
- **GIVEN** managed task access, **WHEN** the user hovers, focuses, or taps the Task Git access help,
  **THEN** it explains that Git calls Kandev's `agentctl` credential helper for attached GitHub
  HTTPS repositories, the helper redeems a scoped broker lease on demand, `gh` uses a broker-aware
  shim, and credentials are not written into the repository.
- **GIVEN** host-specific `github.com` `gh` configuration selects SSH while global configuration
  selects HTTPS. The Kandev-managed GitHub checkout has an HTTPS `origin`. **WHEN** the workspace
  uses executor inheritance and launches or resumes a Local or Worktree task. **THEN** Kandev uses
  the host-specific value. Kandev changes the managed checkout to canonical SSH before task
  preparation, so matching Git conditional includes apply.
- **GIVEN** host-specific `github.com` `gh` clone-protocol configuration is unavailable. **WHEN**
  Local or Worktree preparation constructs an executor-inherited managed-checkout origin. **THEN**
  Kandev uses the supported global protocol. Kandev defaults to SSH when neither scope returns
  `ssh` or `https`.
- **GIVEN** a Local or Worktree task previously launched with one host `gh` clone protocol.
  **WHEN** the host-specific or global protocol changes and the task launches or resumes without a
  Kandev restart. **THEN** origin reconciliation uses the new effective protocol, not the earlier
  process value.
- **GIVEN** a Kandev-managed GitHub checkout currently has an SSH `origin`, **WHEN** the workspace
  selects managed credentials and launches a Local or Worktree task, **THEN** Kandev changes that
  managed checkout's `origin` to canonical HTTPS before task preparation.
- **GIVEN** a Kandev-managed checkout already has the canonical origin selected by the task policy,
  **WHEN** a task launches or resumes, **THEN** Kandev inspects the origin but does not rewrite
  `.git/config`.
- **GIVEN** a task with one or more attached repositories, **WHEN** launch or resume builds the
  primary, multi-repository, and credential configuration, **THEN** each repository is prepared
  once and the same resolved result is reused by all three consumers.
- **GIVEN** a managed checkout is owned by `brewuser` while the Kandev service runs as root,
  **WHEN** Git rejects origin inspection or reconciliation as dubious ownership, **THEN** task
  preparation stops and reports that the service account and managed repository owner disagree,
  without suggesting `safe.directory=*`.
- **GIVEN** Git emits a failure containing credential-bearing URL userinfo, **WHEN** the failure is
  returned through task preparation, **THEN** the diagnostic retains actionable Git context but
  redacts the credential and bounds the output length.
- **GIVEN** a repository is registered from a user-managed local checkout, **WHEN** either task Git
  credential policy is selected, **THEN** Kandev leaves its configured `origin` unchanged.
- **GIVEN** managed mode and an explicit executor-profile GitHub token, **WHEN** a task launches,
  **THEN** the profile token wins and the session disclosure labels its actor runtime-selected.
- **GIVEN** managed mode and an explicit executor-profile `GITHUB_TOKEN` or `GH_TOKEN`, **WHEN** an
  attached repository has incomplete managed identity, **THEN** the profile token remains the
  effective source and managed broker admission does not reject the task.
- **GIVEN** a managed repository remote with an SSH password, query, or fragment, **WHEN** Kandev
  resolves its credential identity, **THEN** resolution fails without exposing the remote and no
  task, workflow, or session state changes.
- **GIVEN** a provider-backed repository whose only usable identity is a local checkout origin,
  **WHEN** a managed task launches or the broker authorizes a request, **THEN** both paths reject
  the incomplete persisted identity before session state changes.
- **GIVEN** task Git credential policy resolution fails, **WHEN** a normal or Office task launches
  or resumes, **THEN** Kandev returns the error and does not create, rebind, or resume a session.
- **GIVEN** a workflow transition selects an agent profile that cannot use managed credentials,
  **WHEN** the source step exits, **THEN** Kandev keeps the source step and current session and does
  not route the destination step prompt.
- **GIVEN** a managed helper cannot execute or redeem its lease, **WHEN** Git requests GitHub HTTPS
  credentials, **THEN** the command fails without falling through to a personal helper or prompt.
- **GIVEN** a broker-enabled managed task whose login profile replaces its inherited `PATH`,
  **WHEN** Git requests GitHub HTTPS credentials, **THEN** the configured helper invokes the
  instance-owned `agentctl` directly and does not search or fall through to an ambient helper.
- **GIVEN** a broker-enabled Local or Worktree task whose checkout or setup script invokes Git
  before the task instance is created, **WHEN** Git requests GitHub HTTPS credentials, **THEN** the
  configured helper invokes the standalone launcher's absolute `agentctl` executable without
  consulting `PATH` or an ambient helper.
- **GIVEN** a broker-enabled Docker or Sprites task whose prepare script clones before `agentctl`
  starts, **WHEN** Git requests GitHub HTTPS credentials during that clone, **THEN** the configured
  helper invokes the already-installed absolute executor binary and redeems the task lease without
  consulting `PATH` or an ambient helper.
- **GIVEN** a broker-enabled managed task with an existing Bash environment hook, **WHEN** a
  non-interactive login shell replaces `PATH`, **THEN** the existing hook still runs and the
  Kandev-managed `agentctl` and `gh` shims are restored ahead of ambient tools before the requested
  command starts.
- **GIVEN** that existing Bash environment hook is expressed as `$HOME/hook.sh` or
  `${KANDEV_HOOK_ROOT}/hook.sh`, **WHEN** Kandev composes its managed startup fragment, **THEN** it
  resolves the reference from the effective child environment and sources the intended hook rather
  than a filename containing literal dollar-sign text.
- **GIVEN** the host or executor exports indexed Git config for `core.hooksPath` and
  `notes.augment.mergeStrategy`, **WHEN** Kandev appends its managed GitHub helper configuration,
  **THEN** the agent receives one contiguous block containing the original entries first and the
  Kandev entries afterward, and a real Git commit still runs the configured hook.
- **GIVEN** Docker or a remote control process already contains the same task Git config suffix,
  **WHEN** that suffix is forwarded again while creating or configuring an agent instance,
  **THEN** Kandev emits it once while retaining executor-added `safe.directory` and URL rewrites.
- **GIVEN** a workspace policy or automation connection changes after launch, **WHEN** the user
  views the running session, **THEN** the Changes disclosure still shows its launch snapshot; a
  successful resume records and shows the newly resolved contract.
- **GIVEN** an authenticated host GitHub CLI without structured status or named-token flags,
  **WHEN** an operator selects its sole account and `gh auth status` reports successfully on
  stderr, **THEN** Kandev discovers and validates that account without requiring a CLI upgrade.
- **GIVEN** a valid migrated `legacy_shared` connection and a legacy shared managed checkout,
  **WHEN** a task needs a workspace-isolated checkout, **THEN** Kandev resolves the same automation
  identity's Git credential and clones into that workspace's managed root without persisting or
  exposing the token.
- **GIVEN** desktop and mobile viewports, **WHEN** users complete every App flow, **THEN** actions and
  disclosures remain usable without clipping, overlap, or desktop-only capability.
- **GIVEN** a mobile coarse-pointer viewport, **WHEN** the user opens Change GitHub connection,
  **THEN** the task access controls share the existing full-height drawer's single scroll owner,
  remain touch reachable, clear the safe area, and introduce no horizontal overflow.
- **GIVEN** desktop fine-pointer and mobile coarse-pointer task views, **WHEN** the branch
  disclosure is opened, **THEN** both show the same credential policy, method, actor truth, and
  transport without horizontal overflow.

## Success Criteria

- No runtime, callback, webhook, cache, broker, or status path resolves a GitHub App without both
  workspace and registration identity where workspace ownership is required.
- A seeded E2E run proves different Apps for work/personal workspaces and intentional App reuse.
- Secret scans find no PAT, private key, client secret, webhook secret, personal token, refresh
  token, or live installation token in logs, API snapshots, redirects, process arguments, or
  executor environments.
- Standalone, container, and remote task tests prove the managed helper is discoverable only for
  broker-enabled instances and their authorized task terminals/processes, while executor
  inheritance receives no Kandev GitHub helper/shim.
- A real Git subprocess test proves that host/executor indexed hooks and notes config survive
  managed credential injection, and focused tests prove ordered composition and overlap handling
  across standalone, container, and remote launch shapes.

## Out Of Scope

- Multiple automation connections or per-repository credential routing inside one workspace.
- Automatically editing an imported App's GitHub settings or uninstalling an App on disconnect.
- Automatic private-key/client-secret rotation; users replace an unbound stored registration or
  import the replacement App credentials through the guided flow.
- GitHub Enterprise Server, enterprise-owned Apps, or hosts other than `github.com`.
- Kandev multi-user login, workspace membership, or RBAC implementation.
- Publishing Apps to GitHub Marketplace.
- Discovering or verifying the actor behind inherited credential managers, SSH agents, or explicit
  profile tokens.
- Preventing a host-authority agent from manually selecting another Git transport outside
  Kandev-injected managed HTTPS and `gh` commands.

## Implementation Plan

See [the original authentication implementation plan](../../../plans/github-authentication/plan.md)
and the
[task Git credential policy follow-up plan](../../../plans/task-git-credential-policy/plan.md), plus
the
[executor clone transport repair plan](../../../plans/github-executor-clone-transport/plan.md), and
the [managed task terminal environment plan](../../../plans/task-terminal-git-environment/plan.md),
and the
[managed GitHub login-shell repair plan](../../../plans/github-managed-tools-login-shell/plan.md),
and the
[system-service identity guardrails repair plan](../../../plans/system-service-identity-guardrails/plan.md).
The new-workspace default repair is tracked in
[the new workspace GitHub access defaults plan](../../../plans/new-workspace-github-access-defaults/plan.md).
Managed credential admission and repository identity corrections are tracked in
[the managed Git credential admission repair plan](../../../plans/managed-git-credential-admission-repair/plan.md).

## Decision

See [ADR-2026-07-21-workspace-selectable-github-app-registrations](../../../decisions/2026-07-21-workspace-selectable-github-app-registrations.md)
and
[ADR-2026-07-27-task-git-credential-policy](../../../decisions/2026-07-27-task-git-credential-policy.md).
New-workspace defaults are defined by
[ADR-2026-08-02-new-workspace-github-access-defaults](../../../decisions/2026-08-02-new-workspace-github-access-defaults.md).
The system-service ownership boundary is defined by
[ADR-2026-07-31-system-service-user-continuity](../../../decisions/2026-07-31-system-service-user-continuity.md).
