---
spec: docs/specs/workspaces/requirements/improve-kandev.md
created: 2026-08-12
status: complete
---

# Implementation Plan: Managed Fork Contributions

## Overview

Keep the Improve Kandev task attached to the canonical `kdlbs/kandev` repository while giving a
managed-credential task one exact, server-verified fork as its publication destination. Resolve that
destination through the workspace automation connection before the first agent launch, persist it on the
canonical task-repository attachment, and reuse the existing remote-contribution patterns for a second
broker lease, a dedicated Git remote, and restart-safe push routing. Preserve executor-owned credentials as
a separate unmanaged path because Kandev cannot prove the identity behind an opaque executor credential.

Review remediation on 2026-08-13 closes the remaining trust-boundary cases: destination credential
bindings carry the selected connection generation, provider IDs are checked against live fork-parent
identity, renamed forks are discovered through the fork network, and bootstrap errors cross the API as
stable reason codes for frontend translation.

The implementation follows
[ADR-2026-08-12-task-bound-fork-destinations](../../decisions/2026-08-12-task-bound-fork-destinations.md),
[ADR-2026-08-04-remote-contribution-bindings](../../decisions/2026-08-04-remote-contribution-bindings.md),
and the [task Git credential policy](../../decisions/2026-07-27-task-git-credential-policy.md).

## Implementation status

- [x] Task 01: contribution-destination model and workspace GitHub resolution.
- [x] Task 02: task-creation preparation, persistence, and bootstrap capability alignment.
- [x] Task 03: managed leases, runtime remotes, push routing, and resume reconstruction.
- [x] Task 04: Improve Kandev workflow, desktop/mobile recovery states, integration coverage, and docs.

## Invariants

- The repository row, task attachment, provider identity, and managed `origin` remain the canonical
  `kdlbs/kandev` repository.
- A fork is one task-bound push destination, not a second workspace source and not an alternate canonical
  repository.
- Only the backend may author `contribution_destination`; REST, WebSocket, MCP, workflow prompts, and Git
  remotes cannot supply a trusted fork identity.
- Fork preparation uses the same workspace automation source that supplies managed task credentials. A
  personal identity is not silently combined with unrelated App credentials.
- A persisted destination records the non-secret credential source/login/generation (and App installation
  identity where applicable). Policy changes, explicit executor tokens, and connection changes clear or
  revoke the managed destination.
- Canonical and target provider IDs are required for managed destination scopes. Lease issuance and
  redemption re-read the target and verify its fork parent through the current automation connection.
- Existing renamed forks are found through the canonical repository's fork network and individually
  re-read before reuse; path-only list results are never trusted.
- All persisted URLs are canonical and credential-free. Tokens, leases, credential helpers, provider
  content, and ambient remote state never enter the binding.
- Managed authorization proves task, session, canonical attachment, binding version, provider, host, and
  exact fork owner/repository, provider IDs, parent identity, and credential binding before issuing the
  second scope.
- Direct target write access needs no fork binding or second lease.
- Executor-owned task access does not receive a workspace-authored destination binding and retains its
  explicit agent-managed publication path.
- Existing `remote_contribution` behavior for already-open pull requests remains unchanged.
- Kandev-managed provider rows continue to reconcile canonical `origin`; user-managed local rows retain
  their current exemption.
- Fork preparation failure occurs before task settlement or agent launch. Runtime validation and resume
  also fail closed on malformed or inconsistent persisted state.

## Backend

### Contribution-destination domain and provider resolution

- Add a versioned `ContributionDestination` beside task repository models. Reuse the existing validated,
  credential-free repository identity shape and deterministic contribution-remote naming where practical,
  while keeping pre-PR bindings distinct from `RemoteContribution`, which requires an existing change
  number, branch, and head SHA.
- Add strict put/load/validate helpers for `task_repositories.metadata["contribution_destination"]`.
  Validation accepts only supported providers and canonical repository URLs and rejects unknown versions,
  credentials, queries, fragments, malformed paths, and target/source identity aliasing.
- Add a narrow GitHub contribution resolver to the workspace GitHub service. It checks direct target write
  access first. Otherwise it requires a human automation principal, checks the exact path and then the
  canonical fork network for a renamed actor-owned fork, proves its parent provider ID/path is exactly
  `kdlbs/kandev`, creates the fork when absent, polls within a bounded deadline, and confirms source write
  access before returning the bound destination.
- Implement the resolver for PAT and named CLI automation clients through their existing authenticated API
  paths. Treat an App installation without direct target write access as unsupported for automatic fork
  ownership and return actionable, typed failure. Never fall back to ambient host `gh`.

### Creation-time preparation and persistence

- Add an optional server-side contribution-destination resolver seam to task creation. It runs after
  external-ID deduplication and workflow/repository validation but before the task insert, so retries reuse a
  settled task and provider failures leave no partly created task.
- The injected Improve Kandev resolver acts only when task Git access is managed, the selected workflow's
  immutable template ID is `improve-kandev`, and the resolved attachment is canonical `kdlbs/kandev`.
  Ordinary workflows, executor-owned access, the issue-only template, local repositories, and already-open
  `remote_contribution` tasks are unchanged.
- Carry the resulting binding only through an internal, JSON-excluded field on
  `service.TaskRepositoryInput`. Validate it again and persist it while creating the canonical
  `TaskRepository`, preserving unrelated metadata and existing PR-number behavior.
- In managed mode, replace Improve Kandev's ambient `gh` bootstrap probe with the same workspace-scoped
  resolver in read-only capability mode. Report direct write, exact ready fork, creatable fork, and blocked
  automation configuration from the identity that will supply task credentials. Keep executor-owned
  capability probing explicitly separate and labeled.
- Persist the canonical provider repository ID during bootstrap for managed and CLI-compatible fallback
  paths whenever the provider returns it. Return stable fork reason codes rather than backend-authored
  English messages.

### Runtime projection and Git routing

- Load the binding into `repoInfo` and thread it through `RepoSpec`, lifecycle prepare/create requests,
  executor metadata, agentctl workspace configuration, and multi-repository projections as one optional
  typed object.
- Generalize the existing contribution-remote setup so a pre-PR destination adds the same
  collision-resistant remote without fetching or checking out a remote head. Preserve the task's current
  feature branch and set its upstream/push target to the same branch name on the fork.
- Reconstruct and validate the fork remote and upstream on fresh launch, warm resume, reset/recreation,
  Worktree, Local, Docker, SSH, and Sprites paths. A conflicting remote identity fails rather than being
  overwritten. Canonical `origin` reconciliation runs as today and does not remove the contribution remote.
- Project explicit push remote/branch data to agentctl so Changes-panel pushes and raw `git push` agree.
  Pull, rebase, merge, base fetch, issue lookup, and change-request target selection remain canonical.

### Credential authorization

- Extend executor managed-scope construction to add the destination source owner/repository after the
  canonical scope. Deduplicate a same-identity result defensively, although valid fork bindings must differ
  from the target.
- Extend the broker authorizer to accept the requested source only when it exactly matches a valid
  `contribution_destination` on the authorized task-repository attachment, including source/target
  provider IDs and a live provider-authoritative parent check. Keep the existing `remote_contribution`
  proof as a separate accepted case.
- Revalidate the destination's credential binding against the current workspace connection and credential
  generation at lease issuance and redemption. Executor-owned policy and explicit profile tokens must
  remove managed destination state before launch.
- Preserve the current credential helper and `gh` shim exact-match behavior. Git push redeems the fork
  lease through the dedicated remote; `gh pr create --repo kdlbs/kandev` redeems the canonical lease. No SSH
  or executor-owned fallback is added to managed mode.

## Workflow and frontend

- Rewrite the Improve Kandev PR-step prompt so managed publication treats routing as already prepared. It
  verifies canonical `origin`, pushes with ordinary `git push` through the configured upstream, and invokes
  `gh pr create` with explicit `--repo kdlbs/kandev`, `--base main`, and `<fork-owner>:<branch>` head
  identity. The executor-owned branch retains the current agent-managed fork setup and is permitted only
  when no managed destination is expected; a managed preparation failure never falls back to it.
- Extend the bootstrap API's fork-status union with a general blocked-managed-credentials state and a safe
  recovery message. Keep issue-only submission available.
- Reuse the existing Improve Kandev dialog and contributor banner on desktop and mobile. This is a
  content/state change inside the current responsive dialog, not a new composition: the same shared model
  blocks Bug fix/Feature request, while Open issue remains the primary recovery path. The existing mobile
  Improve Kandev dialog is the closest exemplar; it retains its single dialog scroll owner, current touch
  targets, safe-area behavior, and absence of horizontal document overflow.

## Tests

- **Domain/provider:** table-driven model tests cover round trip, unknown version, credential-bearing URLs,
  malformed identities, and deterministic remote names. PAT/CLI resolver tests cover direct write, existing
  exact and renamed-network forks, fork creation and bounded polling, same-name wrong-parent conflict,
  source write denial, EMU, App-without-target-write rejection, typed CLI 404 handling, and provider error
  redaction.
- **Creation:** task service tests prove resolution precedes insert/launch, external-ID retries do not repeat
  fork creation, only the Improve implementation template is enriched, internal fields cannot be forged,
  metadata preserves unrelated keys, executor-owned access is bypassed, and resolution failure leaves no
  task or session.
- **Credentials:** executor and broker tests prove canonical plus exact fork scopes for the same task/session,
  reject unrelated forks, wrong workspaces, wrong attachments, malformed bindings, unknown versions,
  changed workspace connections, explicit profile tokens, executor-owned policy, and provider-ID reuse.
- **Runtime:** temporary canonical/fork repositories cover stable remote naming, current-branch upstream,
  direct and agentctl pushes, conflicting remote rejection, canonical-origin reconciliation, local-row
  exemption, restart reconstruction, and unchanged ordinary/remote-contribution behavior on each runtime
  projection seam.
- **Workflow/UI:** loader regression tests pin the managed prepared route, the guarded executor-owned route,
  and the absence of origin rewriting in managed instructions. Component/model tests cover every fork
  status, reason-code translation, task-access mode, and issue-only bypass. Existing desktop and mobile
  Improve Kandev Playwright specs cover the blocked managed-credential message, touch-reachable issue
  fallback, viewport containment, and no horizontal overflow.
- **Integration:** a focused backend test creates a managed Improve task with fake GitHub APIs and temporary
  canonical/fork remotes, launches it, commits, pushes, resumes, and proves only the fork branch advances
  while canonical `origin`, issue identity, broker scope, and PR target stay `kdlbs/kandev`.

## Public documentation

- Update the **Choose task Git credentials** reference section in `docs/public/integrations.md` to explain
  task-bound contribution destinations, canonical `origin`, the exact second lease, and the App limitation.
- Update the Git operations reference only where it describes task-bound contribution push routing. Keep
  the existing Changes-panel `origin` contract for ordinary tasks explicit rather than implying every task
  receives a fork.
- Treat `integrations.md` as reference and `git-operations.md` as reference. No new page or navigation entry
  is required.

## Execution order

1. [Task 01: Destination model and GitHub resolution](task-01-destination-model-and-github-resolution.md)
2. [Task 02: Creation-time preparation and bootstrap](task-02-creation-preparation-and-bootstrap.md)
3. [Task 03: Runtime credentials and fork routing](task-03-runtime-credentials-and-fork-routing.md)
4. [Task 04: Workflow, UI, integration, and docs](task-04-workflow-ui-integration-and-docs.md)

Tasks are sequential. Each task establishes a persisted or security-sensitive contract consumed by the
next, and Tasks 02-03 touch shared creation and launch paths where parallel implementation would create
avoidable merge and trust-boundary risk.

## Completion

Completed 2026-08-12. The implementation keeps the canonical `kdlbs/kandev` attachment and `origin`,
binds one server-verified fork for managed Improve Kandev tasks, carries that binding through launch,
resume, materialization, credentials, broker authorization, and Git/PR operations, and keeps the
executor-owned path separate. The workflow prompt, translated desktop/mobile recovery state, public
references, and targeted browser coverage now describe the same behavior.

The backend integration is covered across the creation, broker, lifecycle, remoting, Git push, and PR
provider seams. The new PR test verifies that only the prepared destination advances and that `gh pr
create` receives the canonical repository and explicit fork-owner head.

## Final verification

```bash
rtk make -C apps/backend test                 # passed
rtk make -C apps/backend lint                 # passed
cd apps && rtk pnpm --filter @kandev/web typecheck # passed
cd apps && rtk pnpm --filter @kandev/web lint      # passed
cd apps/web && rtk pnpm e2e:run --host --no-build --project chromium -- e2e/tests/improve-kandev.spec.ts # 15 passed
cd apps/web && rtk pnpm e2e:run --host --no-build --project mobile-chrome -- e2e/tests/mobile-improve-kandev.spec.ts # 1 passed
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs          # 41 pages validated
rtk git diff --check                               # passed
```

Install `apps/node_modules` with `cd apps && rtk pnpm install --frozen-lockfile` first when the worktree is
fresh. Implementation follows TDD: each task adds its focused failing tests before production changes and
records the red and green commands in its completion section.
