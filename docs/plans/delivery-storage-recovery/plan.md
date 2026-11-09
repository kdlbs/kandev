---
created: 2026-11-09
status: implemented
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/platform/system-design/delivery-storage-diagnostics.md
  - ../../specs/platform/system-design/durable-delivery-failure.md
legacy_specs: []
---

# Implementation plan: Delivery storage recovery

## Overview

Recover truthful storage and session state after a terminal cancellation during storage pressure.
First keep delivery observation active between turns. Then separate observation ages through the existing UI.
Finally converge coarse session state from durable terminal evidence.
All work orders are complete. The design turn authorized no production change; implementation followed in a later explicit request.

Platform owns the package because it owns delivery health, durable terminal settlement, and their session projections.
The user accepted these outcomes and preservation of the same conversation without automatic replay.
No material product choice remains open.

## Evidence and root causes

The November 8 incident has three verified boundaries:

- Delivery reported pressure at 22:59 UTC. The host journal reported `ENOSPC` around 23:10 UTC.
- A cancellation terminal reached projection and settlement around 23:10 UTC. All 4,331 events were later acknowledged, with zero retained event bytes.
- The executor record was `ready`, while the session remained `RUNNING` with cached `interrupted` and paused-producer flags.

The investigation used read-only SQL, backend and host logs, the session API, and an unchanged journal copy.
Direct current agentctl status was unavailable. Therefore, current writer health was not independently established.
Missing ACP records and backend log write losses limit attribution of the original state-write failure.
Committed tests must use synthetic identities and payloads. Private records remain in the current task's ignored diagnostic directory.

`probeDeliveryHealth` returns before status I/O when the submission becomes empty after completion.
`buildRuntimeStorageObservation` copies old pressure while renewing capacity receipt time.
A temporary test reproduced `probes=1`, normal disk state, interrupted delivery, paused producer, and age zero after a hypothetical healthy recovery.
Its name was `TestDebugReproStoragePressureAfterSubmissionSettlement`. The test was removed after diagnosis.

Session settlement has a separate convergence gap.
`setSessionWaitingForInputWithHook` discards a state-update error, while terminal effects can complete independently of coarse session state.
The pending-effect reconciler then ignores completed effects. It cannot repair the observed persisted combination.
Task 03 must reproduce that combination and the retry boundary before implementation.

## Scope

### In scope

- Owner-fenced status observation after the active submission clears.
- Recovery from a confirmed healthy delivery report, without another prompt.
- Independent pressure and capacity ages through DTOs, boot, WebSocket, normalization, and shared presentation.
- A conditional, retryable correction of stale session state from an exact settled terminal.
- SQLite/PostgreSQL parity, successor races, older peers, and desktop/phone proof.
- Public session guidance with the implemented behavior.

### Out of scope

- New storage quotas, reserves, thresholds, retention, cleanup, or journal formats.
- Automatic replay, native replacement, queue dispatch, workflow advancement, or blanket recovery-block removal.
- New recovery controls, overlays, polling registries, feature flags, or state enums.
- Mutation of the reported session, private incident fixtures, or delegated implementation.

## Technical approach

Task 01 changes the existing `StreamManager` observer, not its lifetime owner or polling budget.
Idle probes update pressure only. Active probes retain correlated reconciliation.
Unsupported or obsolete reports supply no clearance authority.

Task 02 adds optional `pressure_observed_at` and `pressure_age_seconds` to the runtime projection and its existing consumers.
Capacity age retains its current meaning. Each successful pressure report refreshes its receipt even when values stay equal.
The browser uses the relevant age and monotonic receipt elapsed time for each claim.
Unknown pressure age is last reported, including with an older backend.
The existing epoch/revision merge remains authoritative across live, boot, and HTTP responses.

Task 03 extends `reconcileAgentDeliverySettlements`, the repository settlement boundary, and existing adoption/recovery hooks.
Use actual receipt and projection proof plus current idle ownership before a conditional session-state write.
An empty pending-effect list cannot exclude a current bound, fully settled terminal from state assessment.
All current-owner submissions participate in successor detection. Existing recovery blocks remain independent.
The repair neither republishes a synthetic completion nor calls the ordinary ready handler to repeat its effects.

The existing decisions preserve enough rationale. No new ADR is required:

- [Independent delivery observation](../../decisions/2026-10-27-all-tool-delivery-boundary.md).
- [Historical outcome and admission separation](../../decisions/2026-10-12-total-session-recovery.md).
- [Prompt-generation ownership](../../decisions/0035-version-agent-ready-events-by-prompt-generation.md).

### Compatibility matrix

| Runtime or peer | Required evidence | Behavior and verification |
| --- | --- | --- |
| Current local/worktree, ACP or native transport | Current client, attachment, harness generation, supported health | Idle status refresh and source-to-DTO tests |
| Current SSH/Docker/Kubernetes | Same identities through existing authenticated executor client | Same observation rules, isolated client fixtures, no backend disk substitution |
| Sprites or nondurable instance | Existing provider-verified advisory capability | Capacity behavior continues. This repair adds no durable-health claim or wake operation |
| Retained peer with recognized legacy health | Supported state and current owned stream | Observe supported live health. Missing storage reasons remain generic |
| Missing, malformed, unsupported, or unauthenticated status | No positive recovery evidence | Preserve last pressure and age it independently. No admission downgrade |
| Old backend snapshot without pressure age | Existing pressure plus unknown freshness | Show last reported. Do not use capacity age as pressure age |
| Settled terminal with a newer live submission | Complete inventory and exact identities | Preserve the successor. Mixed terminal/live regression is mandatory |

## ASCII UI preview

Labels and values are illustrative. Existing localization and controls remain authoritative.
The structural requirements are one notice above the composer, explicit Details, and the same draft and scroll owners.

### UI-01: Pressure freshness, selected task chat or Quick Chat

```text
Desktop, observed defect:
  ! Delivery is blocked on storage | Disk <usage>% used [Storage] [Details]
  Agent is running
  [ Queue instructions ...                                              ]

Desktop, old pressure with a fresh capacity sample:
  ! Last reported: Delivery is blocked on storage       [Storage] [Details]
  [ Details: Pressure observed <time>; capacity observed <time>           ]
  [ Existing composer and recovery restrictions                         ]

Phone, same stale pressure:
  ! Last reported: Delivery is blocked
    on storage                   [Storage] [Details]
  [ Existing composer                              ]
```

Preserve the compact row. Text can wrap within its existing allocation when the locale requires it.
Desktop controls retain normal density. Phone/coarse-pointer controls retain at least 44px targets.
Additional capacity and observation times belong in Details on phones.
Reuse the shipped `SessionDeliveryStorageNotice` and `SessionRecoveryCard` composition, with no new overlay or scroller.
Map UI-01 to criteria 002.2, 002.16, 002.17, 003.5, and 003.6.

### UI-02: Confirmed delivery recovery and settled cancellation

```text
Desktop and phone:
  Chat transcript and historical interruption remain available
  [ Historical recovery card only when an independent cause requires it ]
  [ Write a message ...                                                ]
```

At normal capacity with fresh healthy delivery, remove the live pressure row.
At warning capacity, retain the disk advisory without the blocked-output headline.
Only exact terminal/state reconciliation removes the stale running indication.
Independent blocks can still disable the composer. Recovery never sends its draft automatically.
Background updates preserve focus, draft, transcript position, and native conversation identity.
Map UI-02 to criteria 002.4, 002.15, 003.4, 005.2, 005.3, and 006.5.

## Tests

| Acceptance criteria | Work order and planned evidence |
| --- | --- |
| STORAGE-DIAGNOSTICS-002.1, .4, .5, .7, .15, .17 | Task 01: `TestDeliveryHealthSettledOwnerRecoversWithoutPrompt`, `TestDeliveryHealthIdleProbeRejectsSuccessor`, unsupported-status and Stop/reattachment cases |
| STORAGE-DIAGNOSTICS-002.2, .16, .17 | Task 02: `TestDeliveryStorageCapacityDoesNotRefreshPressureAge`, unchanged-health receipt refresh, DTO and normalization round trips |
| STORAGE-DIAGNOSTICS-003.1, .4, .5, .6, .7 | Task 02: notice component, snapshot ordering, old-peer freshness, desktop/phone Details and recovery presentation |
| DURABLE-AGENT-DELIVERY-005.2, .3 and 006.5 | Task 03: `TestDeliverySessionStateConvergesAfterSettledCancellation`, state-write retry, complete-outbox repair, mixed inventory, and PostgreSQL races |

All criteria use their full `AC-PLATFORM-` identifiers in work orders.
The named tests are implemented permanent regressions covering the required behavior.

## E2E tests

Task 02 extends `delivery-storage-diagnostics.spec.ts` and `mobile-delivery-storage-diagnostics.spec.ts` under `apps/web/e2e/tests/session/`.
Their scoped WebSocket fixtures prove stale pressure during fresh capacity, positive clearance, warning-only capacity, reload, and draft retention.
Backend integration tests separately prove authenticated source-to-projection behavior.
Browser fixture frames alone do not establish that source behavior.

Task 03 extends the existing desktop/phone `resilient-output-delivery.spec.ts` flows through the real backend and mock agent.
Assert settled cancellation, idle session projection, preserved execution/native identity, and exactly one distinct next prompt.
Go fault-injection tests prove the storage-write retry and already-completed outbox repair.
Browser tests must not seed only a desired waiting state and claim settlement proof.
Use causal observers before actions and current-turn markers after acceptance.
Never exhaust the host disk. Run managed `chromium` and `mobile-chrome` commands sequentially.

## Work orders

- [x] [Task 01: Observe delivery health between turns](task-01-post-turn-observation.md).
- [x] [Task 02: Preserve independent pressure freshness](task-02-pressure-freshness.md).
- [x] [Task 03: Converge settled session state](task-03-settled-session-state.md).

Execute 01, then 02, then 03. All work is sequential. Dependency order grants no delegation authority.
Exact commands and owned files are in each work order.

## Companion records

The [storage diagnostics package](../delivery-storage-diagnostics/plan.md) retains its original implemented results.
The [resilient output package](../resilient-agent-output-delivery/plan.md) retains its original completed work orders.
The [session convergence package](../session-recovery-convergence/plan.md) retains native restoration and admission ownership.
This package adds post-turn observation and coarse-state convergence. It does not complete unrelated companion work.

## Verification results

Design-package validation passed on 2026-11-09:

- `python3 scripts/list-docs.py validate`: 439 decisions and 1,946 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: the actual documentation-only diff passed.
  Representative planned source changes also passed, with all three work orders and their requirement/design references accepted.
- Local links resolved across all nine changed Markdown files. Specification files remain within their size limits.
- `git diff --check -- docs/specs docs/plans`: passed.

Implementation verification passed on 2026-11-09:

- Backend race tests passed for the lifecycle, task DTO, orchestrator, backend app, repository, and store-conformance packages.
- `go run -trimpath ./cmd/sqlguard ./internal` and changed-Go lint passed.
- Frontend unit tests, typecheck, targeted ESLint, and the full i18n gate passed.
- Desktop and phone storage diagnostics end-to-end tests passed with fresh artifacts.
- Desktop and phone resilient-output end-to-end tests passed, including a settled cancellation that accepts exactly one distinct next instruction.
- PostgreSQL `DeliverySessionState` concurrency, prepared-successor, and unattempted-reservation tests passed against an isolated PostgreSQL 16 instance.
- `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`, and `git diff --check` passed.
- Requirement/design/spec validation passed after the implementation updates.
- Review follow-up serialized correction with the cancel-in-flight guard, fenced open/reserved successor turns in the writer transaction, propagated session-read failures before outbox completion, validated captured pressure ownership at cache acceptance, and reset the browser pressure-age baseline on each accepted snapshot.
- Added focused regressions for reserved successor turns on SQLite and PostgreSQL, injected session-read failures at both correction reads, stale idle pressure after a successor starts, startup repair of completed outboxes, and rising pressure age across capacity refreshes.

The temporary diagnostic reproduction failed for the expected stale-pressure defect before this package.

## Risks

- A healthy capacity value cannot establish a usable writer or a terminal outcome.
- A late idle status or predecessor terminal can overlap a new prompt on the same execution.
- Retained peers can omit pressure evidence. Unsupported values must preserve uncertainty.
- A state correction can race with cancellation, task archive, workflow movement, or foreground tools.
- SQL and runtime ownership cannot share one transaction. Admission guards and boundary revalidation are required.
- The original incident's exact failed SQL operation remains unproven. The persisted-state regression must stand independently of that attribution.
- PostgreSQL parity and actual browser acceptance were implementation gates; both passed in the implementation turn.
