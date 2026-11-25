---
created: 2026-11-16
status: implemented
requirements:
  - REQ-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001
system_design:
  - ../../specs/integrations/system-design/issue-watch-auto-start.md
legacy_specs: []
---

# Implementation plan: Issue-watch auto-start defaults

## Overview

[Issue #5333](https://github.com/kdlbs/kandev/issues/5333) reports watcher tasks that start only after a user opens them.
One sequential work order repairs the shared launch adapter, records failures, and proves the watcher flow.
The adapter, regression coverage, browser flows, and public reference update are complete.

## Evidence and root cause

Investigation used checkout `b4c6a197129` on 2026-11-16.
The issue had no attachments or comments. GitHub assignment to `carlosflorencio` is verified.

`SentryWatcherSource` carries empty profile strings into task metadata and `AutoStartParams`.
`serviceTaskStarter.Start` forwards those strings directly to `StartTask`.
`resolveEffectiveAgentProfile` supplies step or workflow profiles, but no workspace fallback.
`Executor.prepareSessionAttempt` then returns `ErrNoAgentProfileID` before creating a session.
The coordinator logs the error and retains the task association, without writing the existing task failure marker.

`EnsureSession` uses `resolveTaskAgentProfile`, which includes the workspace default.
That different path explains the reported recovery when the task opens.

A temporary probe drove the real adapter, SQLite repository, scheduler, and executor preparation:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestRepro5333_' -count=1 -v)
```

- `TestRepro5333_EmptyWatcherAgentUsesWorkspaceDefault` failed with `task has no agent_profile_id configured`. The shared resolver returned the available workspace profile.
- `TestRepro5333_WatcherFailureIsVisibleOnTask` failed because `auto_start_failed` was absent after coordinator dispatch.
- `TestRepro5333_EmptyExecutorWithExplicitAgentAlreadyWorks` passed and created one session.

The temporary probe was removed before the design handoff.
It proves the backend defect. It does not claim live Sentry or browser verification.

## Scope

In scope: omitted agent-profile resolution in the shared watcher adapter, existing task failure-marker publication, regression tests, and public profile-default documentation.

Out of scope: retries, old-task repair, required watch fields, watch `last_error`, new UI, new profile policies, and unrelated watcher implementations.

## Technical approach

Change `serviceTaskStarter.Start` in `apps/backend/internal/orchestrator/watcher_dispatch_wiring.go`.
For an empty agent input, load the task and reuse `resolveTaskAgentProfile`.
Return missing-task and repository errors before preparation. When the effective step has an explicit `SessionTarget`, pass an empty profile to `StartTask`, even if task or workspace fallback returned a profile, so target routing selects and validates its source profile first. Without an explicit target, pass the resolved fallback as usual; the existing launch path reports a genuinely unresolved profile.
Preserve nonempty inputs, executor selection, workflow targets, `autoStart=true`, and existing validation.

For each genuine error, reuse `setTaskAutoStartFailedMarker`. Exclude `ErrCeilingLaunchDeferred`.
Return the original error to the coordinator. Do not change reservations, task associations, or poll state.
This confines the repair to the integration adapter and existing task-owned behavior.

Keep the implementation small: one production adapter change, using the existing resolver and marker helper.
Use focused tests for the changed wiring. Reuse existing coverage for the resolver, queue, deduplication, and marker lifecycle.

### Compatibility matrix

| Source and transport | Input shape | Intended behavior | Verification | Unsupported case |
| --- | --- | --- | --- | --- |
| Sentry bus event | Optional agent/executor strings | Resolve omitted agent and retain executor defaults | Real-source coordinator test and browser flow | Missing final profile fails visibly |
| Linear and Jira bus events | Same `AutoStartParams` | Same adapter behavior | Focused adapter tests and existing source tests | Invalid explicit bindings retain validation/self-heal |
| GitLab issue/review bus events | Same adapter, provider-specific associations | Same defaults and failure visibility | Existing source/coordinator regressions | Terminal attachment stops before launch |
| Legacy GitHub, Azure DevOps, plugins | Separate launch paths | Existing behavior | Outside this package | No implied coverage |

Transport-specific launch behavior remains owned by `StartTask`. This repair adds no new ACP or passthrough support.

## Tests

Added `apps/backend/internal/orchestrator/watcher_auto_start_defaults_test.go` with these permanent regression names:

| Criteria | Tests and assertions |
| --- | --- |
| `.1`, `.3` | `TestWatcherAutoStart_UsesWorkspaceDefault`: real Sentry source, no selected profiles, one launched session, expected agent and executor |
| `.2`, `.3` | `TestWatcherAutoStart_PreservesConfiguredProfiles`: explicit watch inputs, step/workflow overrides, and explicit targets to an earlier fixed-profile step despite an unusable workspace fallback |
| `.4`, `.5`, `.6`, `.8` | `TestWatcherAutoStart_ReportsFailure`: table cases for no final profile, task lookup failure, launch error, capacity deferral, and an active sibling |

The failure cases assert the persisted marker and task event. The capacity case asserts the marker remains absent.
Use existing `TestResolveTaskAgentProfile_*` tests for the full resolver precedence and existing coordinator tests for `.4`, `.6`, and `.7`.
Use `TestSessionStartClearsAutoStartFailedMarker` for the marker-clear behavior in `.8`.
These existing tests remain part of the targeted verification command. They do not need duplicate regression suites.

All criteria use prefix `AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001`.
Reuse existing fixtures. Do not append tests to the already-large `event_handlers_test.go` or `task_operations_test.go`.

## E2E tests

Added a shared `watcher-auto-start-flow.ts` under `apps/web/e2e/tests/integrations/`.
Added `watcher-auto-start-defaults.spec.ts` and `mobile-watcher-auto-start-defaults.spec.ts` using that flow.
Projects are `chromium` and `mobile-chrome`, run sequentially.

- For `.1` and `.3`, create a disposable workspace/workflow with an agent default and no step/workflow profile. Create a Sentry watch with both profiles empty.
- Seed issues through `POST /api/v1/sentry/mock/issues?instanceId=...`. Trigger the watch through `POST /api/v1/sentry/watches/issue/:id/trigger?workspace_id=...`.
- Assert the created task has one session with the workspace profile before any task navigation or `session.ensure` request. Then inspect its existing UI.
- For `.4` and `.5`, omit every agent default and trigger a real failure while the board is open. Assert the published marker and `task-state-auto-start-failed` indicator, including after reload.

Reuse the existing kanban indicator and focused phone column. No preview or new UI is required.
The existing `task-auto-start-failed-icon.spec.ts` covers seeded and WebSocket presentation, but does not prove watcher production wiring.
Keep the browser coverage to these two flows, shared between the existing desktop and phone test projects.
Use disposable records and clean them up. Restore any shared fixture mutation in `finally` or `afterEach`.

## Public documentation

Updated the watcher sections of `docs/public/integrations.md` as reference content.
Explain optional agent selection, preserved workflow precedence, workspace fallback, and the existing failure indicator.
Document the affected shared sources precisely. Keep watch poll errors and dispatch failures distinct.
The reference explains optional agent selection, workflow precedence, workspace fallback, executor selection, and failure visibility. Provider polling errors remain distinct from task-start failures.

## Work orders

- [x] [Task 01: Resolve watcher launch defaults and expose failures](task-01-resolve-watcher-launch-defaults.md)

## Verification results

Implementation completed on 2026-11-16. The three diagnostic results appear in Evidence and root cause. The temporary reproduction file is absent. Production changes stay in the existing watcher adapter and use the task profile resolver and conditional failure-marker helper. The user's simplicity constraint was met with three focused Go test functions and existing tests for unchanged contracts.

Verification passed on 2026-11-16:

- `python3 scripts/list-docs.py validate`: 454 decisions and 2,023 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- The work order's local PR-documentation preflight returned `covered`, with no reference errors.
- `git diff --check` passed.
- `python3 scripts/list-docs.py specs --system integrations --text issue-watch-auto-start --format paths` found both new specification files.
- Focused watcher regression tests passed after the expected pre-fix failures. The race-enabled watcher, coordinator, resolver, task-creation, and marker-clear contract set passed.
- The scoped Go lint wrapper passed with `0 issues` in the review follow-up.
- Desktop Chromium passed 4 tests, including the two watcher flows and existing task-marker checks. Mobile Chrome passed 2 watcher-flow tests. Both managed runs built the backend and Vite E2E assets.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed; `node scripts/validate-public-docs.mjs`: 55 pages validated.
- The local PR-documentation preflight returned `covered`, with no reference errors.

Review follow-up verification on 2026-11-16:

- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'TestWatcherAutoStart_|TestCoordinator_|TestWatcherDispatchCoordinator_|TestResolveTaskAgentProfile_|TestCreateIssueTask_|TestSessionStartClearsAutoStartFailedMarker' -count=1)`: passed.
- `(cd apps/web && pnpm e2e:run --project chromium tests/integrations/watcher-auto-start-defaults.spec.ts tests/task/task-auto-start-failed-icon.spec.ts)`: 4 passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/integrations/mobile-watcher-auto-start-defaults.spec.ts)`: 2 passed.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestWatcherAutoStart_' -count=1)`: passed after the adapter correction.
- `bash scripts/lint-go-changed apps/backend/internal/orchestrator/watcher_dispatch_wiring.go apps/backend/internal/orchestrator/watcher_auto_start_defaults_test.go`: `0 issues`.
- `python3 scripts/list-docs.py validate`: 454 decisions and 2,023 specifications validated; `python3 scripts/lint-spec-files.test.py`: 36 tests passed; `python3 scripts/lint-spec-files.py --all`: passed.
- Both managed E2E runs built the backend and Vite assets. `gofmt -l` and Prettier checks passed.

## Risks

- Marking watch selections as explicit selector intent can bypass workflow routing.
- Marking ceiling deferrals as failures can hide queued status.
- A late failure write can mislabel active sibling work. Reuse the conditional marker helper.
- Browser navigation can conceal the defect by calling `session.ensure`. Assert the session before navigation.
- This package does not automatically recover tasks that failed before the correction.
