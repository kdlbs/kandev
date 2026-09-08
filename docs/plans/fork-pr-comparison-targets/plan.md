---
spec: docs/specs/platform/requirements/workspace-git-status.md
created: 2026-08-19
status: implemented
---

# Implementation Plan: Fork PR Comparison Targets

## Overview

Replace the branch-only comparison assumption with a repository-qualified target for tasks that open
or link a pull request from a fork. The exact task attachment owns a credential-free binding to the PR
head and base repositories/branches. Agentctl materializes a deterministic comparison-only remote and
uses its ref for every comparison-derived surface without rewriting `origin`, the checkout upstream, or
push routing.

This repairs the reported case where the fork's stale `origin/main` made Kandev show 562 commits and
`+212980/-16053`, while GitHub PR #1154 correctly showed one commit, three files, and `+133/-29` against
`junhoyeo/tokscale:main`.

Related contracts:

- [Workspace Git Status](../../specs/platform/requirements/workspace-git-status.md)
- [Remote Contribution Tasks](../../specs/tasks/system-design/remote-contribution-tasks.md)
- [ADR: Qualify Git Comparison Targets by Repository](../../decisions/2026-08-19-repository-qualified-comparison-targets.md)
- [ADR: Multi-branch task support](../../decisions/0013-multi-branch-tasks.md)
- [ADR: Bind Fork Push Destinations to Tasks](../../decisions/2026-08-12-task-bound-fork-destinations.md)

## Confirmed root cause

The task stored `base_branch=main` for a repository whose `origin` was the contributor fork. The agent
later fetched and fast-forwarded from upstream, but Kandev's comparison identity never changed:

- `agentctl/server/api/git.go::computeMergeBase` tries `origin/<targetBranch>` before a bare branch.
- `agentctl/server/process/workspace_git_status.go::resolveStoredRef` applies the same origin-first rule.
- Git commit enumeration uses the resulting old merge base with `--first-parent`.
- GitHub PR association persists `TaskPR`/watch state and provider additions/deletions, but does not
  update task-repository comparison metadata, session base SHAs, or live agentctl state.

For the affected worktree, the stored task base produced 562 first-parent commits and the repository-wide
diff. Comparing the same HEAD with the fetched upstream ref produced one commit and the same three-file
diff GitHub reported. No association or refresh error occurred; Kandev simply lacked repository identity
in its comparison contract.

## Invariants

- A comparison target is `(repository identity, branch)`, never a branch plus an implicit remote.
- The exact `task_repositories` row owns the target. Matching requires provider head repository plus
  normalized live checkout branch; repository ID alone is insufficient.
- Provider payloads author the target only after workspace authorization and identity validation.
- Persisted URLs are canonical, HTTPS, and credential-free. Tokens, user remote names, local paths, and
  merge-base SHAs are not persisted.
- `origin`, checkout branch, tracking upstream, contribution remote, and push routing do not change.
- One deterministic `compare-<hash>` remote fetches only the target branch. A conflicting existing
  remote is an error and is never overwritten.
- Once configured, the explicit ref is authoritative for status, task summaries, commits, cumulative
  diff, and Review. Failure never falls back to a same-named origin/local branch.
- Working-tree file status remains available when comparison setup fails; comparison-derived values are
  explicitly unavailable.
- A user selection in **Compare against** atomically clears the provider-derived target.
- Existing credentials are used as-is. This repair adds no private-repository credential scope.

## Backend design

### Domain and persistence

Add `models.ComparisonTarget` beside the existing contribution bindings. Version 1 carries provider,
change kind/number, head repository/branch, and target repository/branch. Reuse the validated
credential-free repository identity shape and deterministic hashing conventions, but use a distinct
`compare-` remote namespace.

Persist the binding at `task_repositories.metadata["comparison_target"]`; no SQL schema migration is
required. Add strict put/load/remove helpers and a narrow merge-preserving repository mutation so an
association cannot clobber unrelated metadata or a concurrent attachment update.

Add a provider-neutral task-service reconciler. Its input is authoritative provider identity, not a PR
URL or user remote. It resolves exactly one attachment by repository identity plus live checkout branch,
persists or updates that row, resets affected session base SHAs, and returns the deterministic per-worktree
projection. Ambiguous/no-match results are typed no-ops: the PR stays associated, but comparison does not
change. Manual base-branch updates remove the binding in the same durable write.

### Agentctl target authority

Extend instance/workspace configuration with a per-repository comparison-target map parallel to
`BaseBranches`. Add an authenticated internal control route and client method that validate bindings,
ensure the deterministic remote URL, fetch only the target branch into its exact remote-tracking ref,
and atomically publish per-repository target state to trackers.

Trackers distinguish three states: no explicit target, ready explicit target, and unavailable explicit
target. Ready targets override branch-only resolution in workspace status, ahead/behind, commits,
cumulative diff, and Review. Unavailable targets keep porcelain/file status but return a bounded error
code for comparison-derived operations. The task status projector carries `comparison_unavailable` so
cards do not render misleading numeric totals.

The initial instance config contains desired targets before polling starts, preventing a transient
origin-based snapshot. The agentctl-ready hook then materializes/fetches them. Live updates use the same
route and trigger detached tracker refreshes.

### Provider association and lifecycle

After GitHub has fetched authoritative PR data and persisted/restored the association, call the task
reconciler with head/base repository IDs, canonical clone URLs, branches, and PR identity. Call the same
reconciler when that PR is retargeted. Only the change identity that authored a target may update it during
background sync; a newly explicit matching association may replace it. Detach clears only its own target;
close/merge retain it.

Wire a DB-backed comparison-target provider and live pusher into lifecycle beside base-branch propagation.
Launch, existing-workspace startup, lazy recovery, restart, and resume hydrate the same map. Repository
subpath keys use the existing branch-identity plan so multi-repository and same-repository sibling
worktrees cannot collapse onto one entry.

GitLab consumes the provider-neutral task-service contract when its MR model supplies equivalent head/base
identity. If current GitLab association lacks complete source identity, keep it a typed no-op in this plan
rather than infer from a URL. Plugin providers remain outside this built-in wiring.

## Frontend and mobile

Add optional comparison identity/error fields to Git status and bounded task-summary types. Changes shows
`<owner>/<repository>:<branch>` in the existing branch details surface on desktop and its existing touch
drawer on mobile. A failed target shows one compact, translated error with recovery guidance; the sidebar
card shows an unavailable indicator instead of additions/deletions.

The existing **Compare against** picker remains the manual escape hatch. Its options are branches from the
attached repository. Selecting one clears the PR-derived target even when its branch name equals the
target branch, so equality checks must compare full target identity rather than only `main`.

This is a state/content change inside the current responsive Changes composition. It adds no new mobile
navigation or parallel desktop-only control. Preserve one mobile scroll owner, touch-sized actions,
safe-area behavior, and no horizontal overflow.

Add translated copy to English, Portuguese, Simplified Chinese, Hong Kong Traditional Chinese, and Taiwan
Traditional Chinese catalogs. Update the reference sections in `docs/public/git-operations.md`,
`docs/public/sessions-and-review.md`, and `docs/public/coordination.md` to explain automatic cross-fork
comparison, manual override, and the unreadable-target failure state.

## Test strategy

- **Domain:** valid round trip, unknown version, credential-bearing URL, unsafe refs, identity mismatch,
  deterministic remote names, metadata preservation, and removal.
- **Attachment reconciliation:** exact head repo/branch match, ambiguous same-repository siblings,
  historical branch, same-repository target, explicit replacement, retarget for same PR, unrelated PR sync,
  detach, and manual override.
- **Agentctl regression:** build a current upstream and stale fork with same-named `main`, then assert status,
  first-parent commits, cumulative diff, and Review all use the upstream comparison ref. Assert no
  `origin/main` fallback after fetch failure or remote collision.
- **Lifecycle:** initial config, agentctl-ready hydration, live fan-out, restart/recovery, multi-repo keys,
  partial per-repository failure, base-SHA reset, event publication, and summary unavailable state.
- **Frontend:** full target display, same-name manual override, cache invalidation, unavailable card/panel
  states, multi-repo scoping, desktop hover, and touch drawer rendering.
- **E2E:** desktop and mobile fixtures reproduce a stale fork plus current upstream, associate a matching PR,
  and assert the one-commit/three-file scope, persisted behavior after reload, manual override, touch access,
  and viewport containment.

Every implementation task starts with the focused failing regression, records the red command, applies the
smallest production change, and records the green command.

## Execution order

[x] 1. [Task 01: Comparison target domain and attachment persistence](task-01-comparison-target-domain.md)
[x] 2. [Task 02: Agentctl comparison remote and authoritative ref](task-02-agentctl-comparison-target.md)
[x] 3. [Task 03: PR reconciliation, lifecycle refresh, and summaries](task-03-pr-reconciliation-and-lifecycle.md)
[x] 4. [Task 04: Desktop/mobile presentation and public docs](task-04-comparison-target-ui-and-docs.md)
[x] 5. [Task 05: Cross-fork desktop/mobile regression](task-05-cross-fork-e2e.md)

Tasks are sequential. Task 02 consumes Task 01's model; Task 03 connects both contracts to provider and
runtime state; Task 04 consumes the status protocol; Task 05 validates the integrated behavior. Parallel
implementation would create avoidable conflicts in shared task-repository, Git event, and Changes-panel
contracts.

## Validation

```bash
cd apps && pnpm install --frozen-lockfile
make -C apps/backend test
make -C apps/backend lint
cd apps && pnpm --filter @kandev/web typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm e2e:run --host --no-build --project chromium -- e2e/tests/git/fork-pr-comparison-target.spec.ts
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome -- e2e/tests/git/mobile-fork-pr-comparison-target.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Completion record

- Backend: `make -C apps/backend test` and `make -C apps/backend lint` pass.
- Frontend: typecheck, lint, focused Changes/task tests (83 tests), i18n checks, and public-doc
  validation pass.
- Cross-fork E2E: the stale fork fixture keeps `origin/main` behind the upstream target; both the
  Chromium and mobile-chrome specs pass with one contribution commit, three PR files, and the
  `upstream/widget:main` target visible in the responsive branch details surface.
- The full `lint:e2e-sleeps` command still reports unrelated pre-existing violations elsewhere in
  the repository; the new E2E files pass the targeted E2E lint configuration.

## Out of scope

- Rewriting `origin`, checkout branches, upstream tracking, or push destinations.
- Automatically retargeting a provider pull request when the local comparison changes.
- Granting new managed credential scopes for private comparison repositories.
- Inferring provider identity from arbitrary Git remotes or a remote named `upstream`.
- Periodic network fetches on every Git-status poll.
- Backfilling historical PR associations without complete head/base repository identity. They reconcile on
  the next authoritative provider read when an exact attachment match is available.
- Plugin-provider implementation. The provider-neutral input remains available for a later adapter.
