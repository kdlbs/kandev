---
created: 2026-10-01
status: implemented
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-007
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
legacy_specs: []
---

# Implementation Plan: Auggie confirmed-mode startup

## Overview

Restore ordinary Auggie startup when the requested legacy mode is already
authoritatively reported by the active session. Implement a guarded adapter
no-op first, then prove lifecycle prompt admission through the real adapter.
Keep strict confirmation for actual mode changes.

The owning requirements and design remain the existing agent permission-control
pair. This is a missing satisfied-selection case in the technical design, not a
new recovery exception. Existing criteria 002.3, 002.7, 007.3, 007.4 and 007.8
remain; added criteria 007.9-007.11 define the unchanged-mode case explicitly.

## Confirmed evidence

At source and running-binary revision `08e4ffdb99caf40b0df5baa67b29cf4313188f15`,
task `eb286cec-581a-4243-bbce-e1c7fd12fe28`, session
`c4e0d0c7-7028-404b-8401-5d94b9363736`, failed on October 1 at
22:23:37 UTC. Its profile selected `gemini-3-7-flash` and `default`.

- The workspace was prepared, the Auggie process started, ACP initialization
  succeeded, Kandev MCP connected, and the selected model was applied.
- Backend log line 17621 in `~/.kandev/logs/backend-logs.log` records:
  `requested permission mode "default" was not confirmed by the agent; the first prompt was not sent`.
- An isolated no-prompt probe of Auggie `0.36.0 (commit 7c61e5bb)` returned
  `modes.currentModeId = default`, advertised `default` and `ask`, and no
  `configOptions`. Requests for `default`, `ask`, and `default` each returned
  `{}` with no mode update within two seconds. The process exited cleanly.
- Installed Auggie's ACP handler sets its session-local mode and returns without
  emitting a current-mode notification. This confirms the observed wire shape;
  it is not authority to infer every silent provider mutation succeeded.
- PR #3886 (`36a2f95e8`, September 28) introduced the required fresh observation
  and startup abort. `awaitModeSettle` intentionally rejects cached pre-request
  evidence after a mutation. The defect is sending that mutation when the
  requested mode is already confirmed, then discarding the useful initial state.

Developer-local raw evidence: `/tmp/kandev-auggie-startup-investigation/` contains
`auggie-probe-20261001-222557.jsonl` and `mode-probe-direct.jsonl`.
These paths are ephemeral, not implementation inputs. The facts above and the
deterministic fixture below are sufficient to reproduce without those files.
Do not copy user transcripts or provider secrets into the repository.

[ACP v1 mode documentation](https://agentclientprotocol.com/protocol/v1/session-modes)
describes initial current-mode state separately from agent-originated updates.
The repair uses reported state, not an invented notification requirement or an
unverified broad success-acknowledgment policy.

## Scope

### In scope

- Confirm an unchanged advertised legacy mode using active, certain provider
  state; skip the redundant mode RPC.
- Preserve the normal confirmed mode event, source attribution, session guards,
  cancellation, config serialization, and prompt gate.
- Cover fresh start, ordinary load/resume, reset, and repeated selection.
- Prove rejection for silent genuine changes and uncertain/stale observations.

### Out of scope

- Treating every empty `session/set_mode` response as confirmation. Auggie
  `default -> ask` remains unconfirmed unless the provider reports the change.
- Relaxing strict Auggie model selection or changing explicit recovery behavior.
- Editing shared settings, adding provider/version exceptions, installing or
  pinning Auggie, adding toggles, or changing CLI flags or auto-approval.
- Redesigning startup-error copy, error classification, or desktop/phone layout.
  The generic UI error is a separate presentation limitation.
- Restarting or mutating the developer's live instance.

## Technical approach

Follow [Already satisfied legacy modes](../../specs/agents/system-design/agent-permission-control-integrity.md#already-satisfied-legacy-modes).

`Adapter.setSessionMode` in
`apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
owns the check. Keep both existing gates, capability validation, active-session
checks, and the shared mode-event epilogue. A small helper may live in
`adapter_mode_state.go` if needed to keep lock ownership and function length
clear. Read certainty and session identity atomically with the observed value.
Do not weaken `awaitModeSettle` or bypass lifecycle enforcement.

There are no schema, API, frontend, localization, or rendered UI changes.
Public documentation review: this design package records an internal repair of
the existing advertised-mode contract. No public command, setting, terminology,
or documented recovery flow changes; no public-doc edit is planned.

### Compatibility matrix

| Provider/shape | Control | Intended behavior | Evidence/fallback |
| --- | --- | --- | --- |
| Auggie 0.36.0, initial `default`, requested `default` | Legacy ACP modes | Confirm observed state, zero mode RPCs, send prompt | Real probe plus deterministic wire/lifecycle fixture |
| Any legacy provider, reported advertised `ask`, requested `ask` | Legacy ACP modes | Same guarded no-op; no provider-name branch | Table-driven adapter test; conditional on certain current-session report |
| Auggie or another legacy provider, current `default`, requested `ask` | Legacy ACP modes | Existing mutation path; silent `{}` stays unconfirmed | Negative wire/lifecycle fixture; first prompt held |
| Claude or other provider advertising a `mode` config option | Config-option ACP | Existing authoritative response and clamp behavior | Existing config-response/clamp tests remain unchanged |
| Custom provider with missing/unadvertised/stale mode or timeout uncertainty | Either | No no-op confirmation; existing validation/error path | Guard, cancellation, reset and uncertainty regressions |
| Explicit provider-restored Auggie recovery | Existing recovery policy | Existing omission behavior remains separate | Existing lifecycle recovery tests |

## Tests

| Criteria | Test evidence |
| --- | --- |
| 007.9, 002.3 | New `TestSetModeAlreadySatisfiedLegacyMode` in `adapter_mode_satisfied_test.go`: confirmed result and normal event, zero RPCs for matching `default`/`ask`, repeated selection |
| 007.10, 007.4 | New `TestSetModeAlreadySatisfiedLegacyModeGuards` in `adapter_mode_satisfied_test.go`: unknown/current-session mismatch, closed adapter, unadvertised value, cancelled caller, preceding unconfirmed operation |
| 007.8-007.11 | New `TestSetModeAlreadySatisfiedLegacyModeSessionTransitions` in `adapter_mode_satisfied_test.go`: new/load/reset replace observations, missing report cannot reuse old mode, matching fresh report qualifies |
| 007.10-007.11 | New `TestSetModeAlreadySatisfiedLegacyModeOrdering`: a request waits behind an actual mutation or transition and rechecks state/cancellation after obtaining both gates |
| 007.10 | `TestLateTimedOutModeReportWhileIdleDoesNotRestoreShortcutCertainty` in `adapter_mode_satisfied_test.go` proves an idle, uncorrelated report cannot clear uncertainty; `TestCorrelatedModeConfigSnapshotClearsLegacyModeUncertainty` covers the correlated recovery evidence |
| 002.7, 007.4, 007.9 | New `TestLegacyConfirmedModeLifecycle` in `session_mode_legacy_integration_test.go`: real lifecycle -> agentctl WS client -> real ACP adapter -> ACP pipe fixture -> prompt callback |
| Existing isolation/clamp guarantees | Existing `TestSetModeUsesAdvertisedModeConfigOptionAndAuthoritativeClamp`, `TestLateTimedOutModeReportCannotConfirmNextRequest`, `TestAwaitModeSettleDoesNotConfirmPreRequestMode`, and concurrent mode/config regressions |

All new cases use TDD. The primary regression fails before the fix because
`SetMode(default)` invokes the silent fixture and returns unconfirmed. Tests
assert mode RPC counts and result identity; they do not merely mirror a boolean
predicate. Keep `awaitModeSettle`'s pre-request-rejection regression intact.

## E2E tests

Backend end-to-end boundary evidence belongs to Task 02. The deterministic
wire/lifecycle fixture uses the real adapter, `SessionManager`, and agentctl
client. The test server routes new/load/mode/prompt actions to the adapter;
it must not return a handcrafted confirmed mode result. Assert exactly one
provider prompt on matching fresh/load flows and zero on different silent modes.
Reset is covered through the real lifecycle-to-adapter fixture, including the
replacement session's own report and missing-report rejection. No Playwright
file/project is added because rendered UI is unchanged.
No real model inference or developer credentials are required.

## Work orders

- [x] [Task 01: Confirm already satisfied legacy modes](task-01-confirm-existing-legacy-mode.md) (`done`)
- [x] [Task 02: Prove lifecycle prompt admission](task-02-lifecycle-wire-regression.md) (`done`, depends on 01)

Execute sequentially. Both work orders are complete.

## Related delivery packages

The [original permission plan](../agent-permission-control-integrity/plan.md),
[PR #3886 remediation](../agent-permission-pr3886-remediation/plan.md), and
[session-control replacement](../agent-permission-session-controls/plan.md)
remain historical/current records of their own work. This package adds only the
missing unchanged legacy-mode case. It does not reopen their completed tasks or
claim that the outstanding real-Claude permission evidence is complete.

The [strict Auggie startup work order](../agent-resume-mode-fallback/task-01-strict-auggie-startup.md)
and [explicit recovery contract](../../specs/agents/requirements/explicit-resume-settings.md)
remain authoritative for task model policy and recovery. Their refusal,
mismatch, and genuinely unconfirmed-mode checks remain required.

## Verification results

Design-package checks passed on October 1, 2026:

- `python3 scripts/list-docs.py validate`: 339 decisions and 1281 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` exported `validateCoverage` preflight with the
  package's three Markdown files, the requirement/design pair, and the planned
  adapter source path: `covered`, both work orders accepted, zero errors. This
  is planned-source coverage, not a claim that production code changed.
- New package relative Markdown targets exist; `git diff --check -- docs/specs
  docs/plans/auggie-confirmed-mode-startup` passed.
- `git status --short` confirms the two existing spec edits and the untracked
  package directory. Everything remains unstaged and uncommitted.

## Implementation results

Both work orders are complete. Task 01 adds a session-scoped uncertainty rule:
an uncorrelated legacy-mode report cannot restore certainty after an
unconfirmed operation, including when the report arrives while idle. A
correlated mode-config response or a fresh report from a replacement session
can restore certainty. Task 02 proves prompt admission through the lifecycle,
agentctl WebSocket client, production ACP adapter, and ACP pipe fixture,
including reset replacement and missing-report behavior.

The lifecycle fixture was red with the shortcut disabled for fresh start,
native load, and context reset, and green after restoration. Work-order checks
passed, including the adapter package suite and race regressions recorded in
Task 01, the lifecycle and reset matrices, the lifecycle fixture under `-race`,
`make -C apps/backend build`, the docs catalog and specification lint, and
`git diff --check`.

Code-review follow-up moved the new satisfied-mode and idle late-report tests
to `adapter_mode_satisfied_test.go`. The PR's changed test files are 493 lines
(`session_mode_legacy_integration_test.go`), 402
(`adapter_mode_satisfied_test.go`), and 671 (`adapter_mode_ordering_test.go`),
each below revive's 800-line limit. The unchanged `adapter_mode_set_test.go`
remains 762 lines. The queued-mode regression waits for a signaling context to
prove the successor's mode-gate try-lock failed while the first operation was
held, asserts no early result or second RPC, then verifies both results and one
provider mutation.
The regression failed as expected under a temporary overlay that cached mode
state before the gate; the overlay was removed.
The full ACP adapter suite and race target passed after these test changes, and
the queued-mode regression passed 20 consecutive runs.

Planning-time publication checks on main revision `994807230d7a03385dc80b001d8a2fd6f8adb4f3`:
the package applies cleanly; the catalog validates 339 decisions and 1283
specifications; specification lint, all 36 linter tests, package coverage
preflight, and the normal pre-commit and commit-message checks passed.

## Risks

- A naive cached-mode shortcut can erase timeout uncertainty or confirm a value
  from a replaced session. Atomic identity/certainty guards and negative tests
  are mandatory.
- An early return can skip mode events or their generation/source ordering.
  Preserve the common completion path and assert emitted state.
- This fixes the reported default-mode startup, not Auggie's silent genuine mode
  changes. Keep that limitation visible in the implementation handoff.
- The wire fixture must exercise real adapter results; a prefilled WS response
  would miss this exact regression.
