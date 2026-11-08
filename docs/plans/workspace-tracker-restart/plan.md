---
created: 2026-11-08
status: complete
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-tracker-restart.md
  - ../../specs/platform/system-design/workspace-git-status.md
legacy_specs: []
---

# Implementation Plan: Workspace tracker continuity after agent restart

## Overview

Restore Changes updates after an agent startup retry on a retained agentctl instance.
First repair stopped-tracker replacement in the process manager.
Then prove the actual retry, stream, and rendered diff path on desktop and phone.
Both work orders run sequentially in the primary session.

This package is implemented. The retained-manager tracker renewal and both browser recovery proofs pass.

## Confirmed diagnosis

The affected task was `e832f5f3-9a95-4b74-94f0-9133444e2069`.
Its session was `6abe9916-f287-4470-a86b-6fbb21f6ba7d`.
The edit to `managed_npm_runtime_versions.json` remained on disk: Copilot `1.0.105` changed to `1.0.106`.
The diagnostic baseline contained 32 Git-status failures for that session. Every failure had class `context_canceled`.
At `2026-11-08T12:40:45.950Z`, managed-runtime startup recovery stopped the workspace tracker before retry.
The replacement agent started, but its tracker retained the canceled lifetime.
The baseline ZIP had a retention-limit warning. These incident records were present within the retained range.

A temporary Go overlay reproduced the defect through the real process-manager lifecycle:

1. Create a disposable Git repository and modify a tracked file.
2. Start the process manager and accept a fresh status containing that file.
3. Call Stop, then Start on the same manager.
4. Request fresh status twice with uncanceled caller contexts.

`TestDebugGitStatusAfterAgentRestart` reported the same tracker pointer and `context canceled` after the successful restart.
Both final refresh assertions failed for that reason.
The overlay and diagnostic copies were removed. The user's runtime and task files were not changed.

## Requirement reconciliation

The [active workspace Git requirement](../../specs/platform/requirements/workspace-git-status.md) already covers the required outcome.
This is an implementation regression against criteria `.2`, `.5`, `.21`, `.27`, `.29`, `.31`, and `.43`.
No new requirement ID or copied requirement is necessary.
The [design supplement](../../specs/platform/system-design/workspace-tracker-restart.md) defines the missing restart mechanics.

Platform owns tracker lifetime and publication. Agents owns managed-runtime recovery policy.
The [managed-runtime design](../../specs/agents/system-design/managed-npm-runtime-recovery.md) remains an adjacent dependency.
The [instance-stop contract](../../specs/platform/system-design/agentctl-instance-stop.md) retains full cleanup and retryable teardown errors.
The existing [publication ADR](../../decisions/2026-09-30-progressive-workspace-git-refresh.md) remains binding.
No material product choice is unresolved.

## Scope

### In scope

- Replace terminal trackers before a new agent starts on the retained manager.
- Preserve root, named repository, initialized submodule, lazy-read, configuration, and subscriber behavior.
- Preserve tracker shutdown cleanup, publication ordering, and stale-source rejection.
- Prove both fresh HTTP reads and existing-stream delivery after restart.
- Prove the restored Changes outcome through actual startup recovery on desktop and phone.

### Out of scope

- Changing provider/runtime selection, npm recovery, attempt counts, or prompt admission.
- Increasing refresh timeouts, retry frequency, or background polling.
- Adding UI controls, copy, schema changes, feature flags, or new metrics.
- Restarting or modifying the affected live task as a planning action.

## Technical approach

`Manager.stopWorkspaceTrackers` remains the cleanup authority.
`WorkspaceTracker.Stop` remains terminal. Reopening its context or resetting its one-time stop guard is not the repair.

Add a focused helper in `apps/backend/internal/agentctl/server/process/manager_workspace_restart.go`.
Integrate it at the common `startWithGeneration` boundary before process creation.
Use the current graph builder and `configureTracker`, with a live-tracker condition on root and repository reuse.
Invalidate stopped lazy trackers without adding their polling loops.
Serialize renewal with existing lifecycle admission and `rescanMu`.
Publish references under existing graph locks and reattach current subscribers without closing their channels.
Update the process runner's tracker reference and preserve effective polling mode.

Use new tracker identities and fresh observations for replacement sources.
Keep unchanged live trackers, callback client leases, environment validation, and capture-time ordering.
Cover unsubscribe races and terminal shutdown with the race detector.

### Compatibility matrix

| Runtime or scope | Intended behavior | Evidence | Unsupported or failed shape |
| --- | --- | --- | --- |
| Persistent ACP subprocess | Fresh tracker after Stop/Start on the same manager | Process regression and actual startup-retry browser flow | Startup failure keeps existing recovery rules |
| One-shot adapter | Common start boundary restores selected stopped trackers | Process test using an existing one-shot fixture | Unsupported adapter setup retains its existing error |
| Live workspace-only promotion | Retain healthy trackers and existing stream | Existing promotion tests plus identity assertions | Replaced execution/client remains rejected |
| Git root, named repositories, initialized submodules | Preserve valid scopes, configuration, and anchors | Graph, API, and stream regressions | Missing repository stays unavailable |
| Lazy subpath reads | Recreate stopped cached entries through existing validation | Lazy-read regression | Invalid or revoked paths remain rejected |
| Local worktree | Preserve edit, index, refs, execution, and session | Browser worktree fixture and real Git assertions | No content mutation to repair reads |
| Docker, SSH, and other executors using agentctl | Share the process-manager correction | Shared Go coverage, not new executor-specific browser certification | Existing executor startup and teardown contracts apply |
| Final teardown or passthrough-only cleanup | Drain tracker resources and prevent resurrection | Existing stop tests plus lifecycle race coverage | Failed cleanup remains an error |

## Tests

All new names below are covered by permanent tests.
Task 01 first reproduced the expected failure without the production correction.

| Criteria | File and tests |
| --- | --- |
| `.2`, `.19`, `.43` | `manager_workspace_restart_test.go`: `TestManagerWorkspaceRestartFreshStatus`, `TestManagerWorkspaceRestartExistingSubscriber` |
| `.21`, `.27`, `.43` | Same file: `TestManagerWorkspaceRestartLiveGraph`, `TestManagerWorkspaceRestartMixedGraph`, `TestManagerWorkspaceRestartRetiresOldPublication` |
| `.2`, `.31` | Same file: `TestManagerWorkspaceRestartScopes`, `TestManagerWorkspaceRestartLazyTracker`, `TestManagerWorkspaceRestartConfiguration` |
| `.5`, `.31` | Same file: `TestManagerWorkspaceRestartCleanup`, including pre-start Stop, repeated cycles, canceled waiter, unsubscribe, and terminal shutdown |
| `.2`, `.19`, `.21` | `api/git_status_restart_test.go`: `TestGitStatusAfterManagerRestart` for single and multi routes, fresh membership, then details |
| `.27`, `.43` | Existing `streams_callbacks_test.go`, `streams_workspace_promotion_test.go`, and process promotion/rescan tests remain green |

Persistent and one-shot starts are explicit subtests in the lifecycle regression.
Use disposable repositories and bounded channels rather than timing sleeps.
Include one healthy named scope beside one stopped scope, plus a failed current repository beside a healthy repository.
Success requires all current healthy scopes to refresh. A failed current scope remains unavailable.

## E2E tests

Task 02 owns two new files:

- `apps/web/e2e/tests/session/managed-runtime-git-restart.spec.ts`, project `chromium`.
- `apps/web/e2e/tests/session/mobile-managed-runtime-git-restart.spec.ts`, project `mobile-chrome`.

Reuse `prepareManagedRuntimeProfile` with a silent first exit, `hostSubprocess: true`, and the existing retry-release barrier.
Choose the disposable worktree executor and record the canonical worktree path and retained execution identity.
Keep Changes mounted while releasing attempt two.
After recovery succeeds, change one tracked file through the filesystem and observe its real membership and ready patch.
Capture the resulting workspace notification before accepting the DOM assertion.
Hold competing foreground Git reads for this assertion window so polling recovery cannot conceal broken stream delivery.
Then release reads and assert a successful correlated fresh response.

Both tests cover `.2`, `.19`, `.20`, `.29`, and `.43`.
They retain the same task/session/execution, one accepted prompt, two startup attempts, and existing source-validation rules.
Phone uses its existing Changes navigation and diff dismissal.
Cleanup releases barriers, restores modified tracked content and shared fixture state, and removes disposable task/profile resources in `finally`.

There is no rendered composition or copy change, so an ASCII UI preview is not applicable.
The desktop exemplar is `workspace-stream-promotion.spec.ts`.
The phone exemplars are `mobile-workspace-stream-promotion.spec.ts` and `mobile-managed-runtime-startup-retry.spec.ts`.

## Work orders

- [x] [Task 01: Restore tracker lifecycle after agent restart](task-01-tracker-lifecycle.md)
- [x] [Task 02: Prove Changes recovery after startup retry](task-02-startup-retry-proof.md)

Task 02 depends on Task 01. No delegation is authorized.

## Verification commands

Run each block from the repository root. Work-order files define the expected assertions.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(BuildWorkspaceCallbacks|WorkspaceStreamPromotion|ConnectWorkspaceStream|RetryManagedRuntimeStartupLifecycle)' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/session/managed-runtime-git-restart.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-managed-runtime-git-restart.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the two browser commands sequentially with fresh managed builds.
Do not overlap full E2E suites or change the runner's worker budget.
The full process race suite takes about 11 minutes; add `-timeout 15m` on hosts with the default 10-minute Go test timeout.
Task 01 also requires changed-file Go lint. Task 02 owns focused web lint and typecheck.

## Verification results

- RED gate: `TestManagerWorkspaceRestartFreshStatus/persistent` failed with `get fresh status ... context canceled` when renewal was disabled before the production edit.
- Process regression: `go test -trimpath ./internal/agentctl/server/process -run '^TestManagerWorkspaceRestart' -count=1` passed.
- Process race suite: `go test -trimpath -race ./internal/agentctl/server/process -count=1 -timeout 15m` passed in 565.993s.
- API race suite: `go test -trimpath -race ./internal/agentctl/server/api -count=1 -timeout 15m` passed in 218.883s.
- Lifecycle callback regressions: `go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(BuildWorkspaceCallbacks|WorkspaceStreamPromotion|ConnectWorkspaceStream|RetryManagedRuntimeStartupLifecycle)' -count=1` passed in 2.405s.
- Changed Go lint: `bash scripts/lint-go-changed ...` reported `0 issues`.
- Desktop browser proof: `pnpm e2e:run --project chromium tests/session/managed-runtime-git-restart.spec.ts` passed in 19.1s (test 16.3s).
- Phone browser proof: `pnpm e2e:run --project mobile-chrome tests/session/mobile-managed-runtime-git-restart.spec.ts` passed in 17.9s (test 14.1s).
- Existing desktop and phone workspace-stream-promotion scenarios passed after the correlation trace extension.
- Deterministic subscriber handoff regression: `TestManagerWorkspaceRestartSubscriberHandoffRace` covers publication, concurrent unsubscribe, retirement, delayed old-tracker publication, and delayed subscribe for reconcile and rebind. The focused handoff race command passed in 21.361s.
- Browser negative control: with renewal disabled, the desktop scenario failed at `Waiting for the restart file membership workspace stream event` while agentctl logged `context_canceled` Git failures.
- Focused web lint and `pnpm run typecheck`: passed.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`: passed.
- Implementation files remain unstaged and uncommitted. The active requirement remains unchanged.

## Companion packages and documentation impact

The implemented [workspace stream continuity package](../workspace-stream-continuity/plan.md) repairs backend callback leases.
It does not recreate stopped agentctl trackers. Its completed work orders remain unchanged.
The implemented [progressive refresh package](../changes-panel-git-refresh/plan.md) retains snapshot quality and ordering authority.
The [loading-feedback package](../changes-loading-feedback/plan.md) owns toolbar status and delayed recovery.
This package changes none of their acceptance criteria, delivered scopes, or recorded validation results.
Exact-path, plain-patch, external-diff, and rename-Discard packages linked by the requirement remain independent.

Internal docs only. Public procedures, settings, wire formats, and user-facing terminology do not change.

## Risks

- Reusing a tracker by path alone leaves the canceled lifetime selected.
- Rebuilding every live tracker discards useful identity and creates unnecessary work.
- Losing subscribers or poll mode makes reads pass while live delivery still fails.
- Reusing old caches or revisions can admit stale patch content.
- Clearing only root trackers leaves named or lazy reads broken.
- Removing Stop cleanup leaks passthrough, observation, enrichment, or index resources.
- A synthetic browser status response can pass without exercising the defect.
