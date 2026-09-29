---
created: 2026-09-28
status: complete
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-001
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
legacy_specs: []
---

# Implementation plan: Workspace ACP idle suspension

## Overview

Implement the user's clarified workspace policy for issue #4006 across all ACP agents.
The setting is disabled by default with a 120-minute timeout. Resume on messages or explicit focus, then suspend again after another idle interval.
The earlier OpenCode-only quiescence and macOS smoke gates are removed. They do not block this package.

## Evidence and scope correction

At revision `adc5d67f55d19dd76161eaec5ae725e20a345acb`, attached streams keep `Instance.IsIdle` false.
The backend `classifyIdleReclaim` separately skips live runtimes. Existing focused tests confirmed both behaviors.
These guards remain valid for disconnected reaping and dead-row repair; neither implements the requested workspace policy.

The implementation agent investigated the earlier provider gate and added no production or permanent test code.
Its audit remains in Task 01 as historical evidence only. The user explicitly replaced that gate with a general opt-in retention policy.
This revision supersedes the previous one-hour installation policy, standalone-only scope, and message-only recovery design.

## Scope

### In scope

- Workspace persistence/API/UI: enabled=false and timeout=120 minutes, with immediate updates and tenant isolation.
- All ACP providers through shared session state and known-work checks, without provider quiescence APIs.
- Exact-process shutdown that preserves task compute, conversation identity, workflow, and workspace.
- Durable idle-suspension provenance, shared message/focus resume, concurrency protection, and repeated suspension cycles.
- Desktop/mobile settings, localization, focus behavior, E2E coverage, and operator documentation.

### Out of scope

- Passthrough terminals, taskless run-owned sessions, resident admission ceilings, and provider-internal background-work reconstruction.
- Reinterpreting `KANDEV_ACP_IDLE_TIMEOUT` or requiring new provider endpoints.

## Technical approach

Task 01 adds a shared runtime suspension primitive and durable suspension provenance.
Task 02 adds workspace policy persistence, settings, scheduling, and all recovery callers as one functional slice.
Task 03 proves complete cycles and documents the policy.

Use the [design](../../specs/executors/system-design/idle-runtime-parking.md) for identity claims, clock reset rules, migrations, and recovery ordering.
Use existing workspace authorization, partial updates, and SQLite/PostgreSQL repository patterns.
Reconcile the narrow focus exception in the existing prevent-auto-start-on-open requirement; do not bypass workflow ownership.

| Boundary | Policy | Verification |
| --- | --- | --- |
| OpenCode and other ACP providers | Identical eligibility; use existing resume/load | Varied-provider table tests, no allowlist |
| Local and remote/container executor adapters | Release agent process; retain task compute | Per-adapter stop-reason and restore contract tests |
| Missing resume token or unsupported restore | Preserve runtime and diagnose | Negative capability tests |
| Passthrough/manual stop/workflow park | No idle-focus recovery exception | Exclusion tests |

## ASCII UI preview

UI-01: Workspace settings > Overview > Resource saving. Default state shown.

```text
Desktop
+------------------------------------------------------+
| Resource saving                                      |
| Suspend idle ACP agents                    [Off]     |
| Idle timeout (minutes)            [120, disabled]     |
| Resume when you open the task or it gets a message.   |
|                                            [Save]    |
+------------------------------------------------------+

Phone
+----------------------------------+
| Resource saving                  |
| Suspend idle ACP agents    [Off] |
| Idle timeout (minutes)           |
| [120, disabled                 ] |
| Resume when you open the task    |
| or it gets a message.            |
| [Save                          ] |
+----------------------------------+
```

When enabled, the timeout field becomes editable. An invalid timeout shows an inline error and prevents saving.
Save failures retain edits and expose the existing retry pattern; pending save disables duplicate submission.
Labels, field order, and behavior are structural; spacing and English copy are illustrative and require localization.
Phone uses the existing page scroll owner, 44px touch targets, and stacked controls. Desktop uses 28px controls.
The entry point is the existing workspace overview; no drawer or new navigation layer is needed.
This preview maps to AC-EXECUTORS-IDLE-PARKING-001.1 and .9.

## Tests

| Criteria suffix under AC-EXECUTORS-IDLE-PARKING-001 | Implemented coverage |
| --- | --- |
| .1, .9, .10 | `TestWorkspaceIdlePolicyDefaultsAndPartialUpdates`, `TestWorkspaceIdlePolicyMigrationDefaultsExistingRows`, `TestPostgresIdleSuspensionPolicyAndProvenance`, and workspace idle-policy component tests |
| .2, .3, .7, .8 | `TestSuspendIdlePreservesRuntimeOwnershipWithoutAgentStopped`, `TestSuspendIdleRejectsStaleIdentityAndMissingRestoreData`, stale lifecycle upsert regression, and executor stop-contract tests |
| .2, .6 | `TestIdleParkingSuspendsSettledSessionsAcrossACPProviders`, `TestIdleParkingKeepsDisabledAndKnownWorkSessionsRunning`, and fake-clock policy/focus tests |
| .4, .7 | Mobile E2E sends a message while suspended and verifies one user message and one content reply; existing resume-attempt serialization remains the shared concurrency owner |
| .5, .7 | `TestFocusTaskSessionResumesIdleSuspensionWithNewLSPLease`, workflow/manual-stop exclusion tests, and session-resumption hook tests |
| .5, .6 | Chromium E2E resumes on explicit focus without a prompt and observes another suspension after a fresh interval |

The backend regression suite also covers stale suspension identities, retained workspace/task resources, and late lifecycle writes. The desktop and mobile E2E flows use the mock ACP runtime and actual lifecycle persistence.

## E2E tests

- `tests/session/session-idle-parking.spec.ts`, chromium: defaults/save, persisted suspension, prompt-free focus recovery, stable transcript content, and repeated suspension after a fresh interval.
- `tests/session/mobile-session-idle-parking.spec.ts`, mobile-chrome: defaults/save, message-triggered resume while parked, and exactly-once touch-composer delivery.
- Backend tests cover disabled defaults, workspace isolation, known active work, provider-family coverage, migration, and manual/workflow stop exclusions.

Use the mock ACP agent for reproducible lifecycle evidence and owned-process tests for actual resource release.
A real-provider smoke can supplement evidence; no OpenCode-specific or macOS-only smoke blocks implementation.

## Work orders

Sequential execution; no delegation is authorized.

- [x] [Task 01: Shared ACP suspension primitive](task-01-conditional-parking.md) (done)
- [x] [Task 02: Workspace policy and automatic recovery](task-02-idle-policy.md) (done)
- [x] [Task 03: Complete-cycle validation and documentation](task-03-resume-validation.md) (done)

## Verification results

Earlier investigation passed:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/instance -run 'TestIsIdle_RespectsInflightRequests|TestActivityMiddleware_BumpsAndDecrements' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestIdleReaper_TickSkipsLiveRuntime$' -count=1)
```

Implementation validation passed:

- `CGO_ENABLED=1 go test -tags fts5 ./...`
- `make -C apps/backend build` and `make -C apps/backend lint`
- `go test -race -tags fts5 ./internal/orchestrator -run '^(TestFocusTaskSession|TestIdleParking|TestIdleSessionFocus)' -count=1`
- PostgreSQL 16 `TestPostgresIdleSuspensionPolicyAndProvenance`
- Web typecheck, i18n validation, and Vite production build
- Chromium and mobile-chrome idle-suspension E2E tests
- Package catalog, specification lint, public documentation validation, and whitespace checks

The Chromium test observed a persisted stop, a focus-only resume without provider prompting, and a later stop after another full idle interval. The mobile test observed a message-triggered resume while the session remained parked in the UI, followed by exactly-once delivery from both the message API and the touch composer.
Issue #4006 remains assigned to `carlosflorencio`.

PR fixup closed a startup-status/focus overlap by making focus join the pending startup recovery for the same request generation. A deferred frontend regression proves the overlap produces one status request and one `session.launch`; the fix satisfies the existing concurrent-resume design rule, so no durable contract changed. The workspace policy card and session-resumption suites passed together (40 tests), along with changed-file ESLint, web typecheck, documentation catalog validation, specification lint, and whitespace checks. No new mobile E2E was needed because this is shared lifecycle state with no change to layout, touch, navigation, or scrolling; existing mobile idle-suspension coverage remains applicable.

The current-head mobile E2E then exposed an unintended gate: workspace eligibility required an OS process-descendant probe to return `settled`, contrary to AC-EXECUTORS-IDLE-PARKING-001.2 and the system design. Removed that prerequisite and retained Kandev's tracked background-work registry as the known-work guard. The new regression proves the idle policy does not call the process probe even when it would report live, unknown, or error; a separate case proves tracked background work still protects the session. The mobile E2E now includes the persisted executor/session/workspace snapshot in its suspension wait and passes through message wake and touch-composer delivery. Focused Go, race, build, and mobile E2E results are recorded in Task 02.

## Risks

- ACP does not expose every provider-internal activity. The opt-in policy uses Kandev-observed state and protects known work.
- Generic stop events can cancel sessions; suspension needs distinct provenance and identity-checked event handling.
- Ordinary executor stop can destroy task compute; suspension must preserve it across each executor adapter.
- Focus callbacks can duplicate or originate from hidden tabs; both client filtering and backend deduplication are required.
