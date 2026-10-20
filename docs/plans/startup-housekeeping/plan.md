---
created: 2026-10-19
status: implemented
requirements:
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-001
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-002
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-003
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-004
system_design:
  - ../../specs/platform/system-design/startup-housekeeping.md
legacy_specs: []
---

# Implementation plan: startup housekeeping and restoration

## Overview

Deliver one PR that removes repeated historical scans, completes required global
housekeeping before normal activation, exposes the work and timings in existing
startup screens, and restores independent conversations without a serial global
queue. After the design handoff, the user authorized implementation and delivery
in one change, including CI/review fixes and merge without further confirmation.

Platform owns this package because it owns readiness, startup measurements, and
shared restoration. The [requirement](../../specs/platform/requirements/startup-housekeeping.md),
[design](../../specs/platform/system-design/startup-housekeeping.md), and
[decision](../../decisions/2026-10-19-startup-housekeeping-before-activation.md)
are the authoritative package. Earlier startup lifecycle, progress visibility,
startup-log-correction, and durable recovery packages remain historical delivery
records; this package extends their mechanisms rather than reopening their tasks.

## Scope

### In scope

- Outstanding-row prompt-sequence repair and missing-counter-only seeding.
- Filtered authoritative runtime classification without deleting resume tokens.
- Explicit local preparation before launch-capable producers and restoration.
- Named housekeeping steps, completed execution durations, and frozen total time.
- Bounded concurrent, deduplicated, promptless restoration with focus priority.
- Shared backend/web localization, desktop and phone rendered proof, public docs.
- Existing targeted checks followed by commit, push, PR, and actionable CI/review
  remediation under the already authorized delivery request.

### Out of scope

- Live-instance restart, SQL repair, journal mutation, or process termination.
- Changing survival flags, backup retention, provider support, or scheduled work
  policy outside the restoration operation.
- A migration framework, new progress endpoint, stored timing history, or new
  native desktop shell progress bridge.

## Technical approach

Task 01 restricts replay to outstanding state in `sqlite/base_migrations.go`,
preserving raw legacy fixture support, ordinal semantics, counters, and both
database dialects. No global success marker is introduced.

Task 02 splits preparation from activation across `backendapp/main.go` and
`orchestrator/service.go`, auditing constructor side effects. Add named steps
around the actual queue/turn/route/runtime/task preparation operations. Remote
adoption-dependent state repair stays session-scoped; it is unsafe to move the
current whole reconciliation function before adoption without separating it.

Task 03 replaces the serial background restore loop with a process-owned
two-operation dispatcher. Existing promptless native restore and checkpoints
remain authoritative. Focus uses the existing activation source and joins a
matching attempt rather than issuing a competing restore.

Task 04 extends the startup reporter and wire parser with bounded per-activation
history, then renders it in the existing Go page, React restart dialog, and
launcher. Extend existing locale/completeness tests and the original registry
contract together. Keep every consumer's current boot/sequence ordering behavior.

| Shape | Expected behavior | Evidence and fallback |
| --- | --- | --- |
| SQLite fresh, upgraded, interrupted | Same ordinals and nondecreasing counters; no repeated full-history aggregation | Real repository replay and query-plan tests |
| PostgreSQL fresh and replay | Same repair semantics in isolated schemas | DSN-backed integration tests; skipped tests are not evidence |
| Local ACP/native resumable conversation | Same identity, guarded promptless restoration | Two-session concurrency and no-dispatch regressions |
| Surviving SSH/container/plugin remote | Existing authenticated attachment; ownership remains session-scoped | Fake blocked remote plus existing provider recovery tests; no new support implied |
| Unknown owner, unsupported native restore | Existing pending/blocked outcome | No replacement or prompt submission |
| Passthrough, Office, automation | Existing eligibility and independent policy | Excluded from automatic chat-native restore when already excluded |

## ASCII UI preview

UI-01: Existing bootstrap startup page and Settings restart dialog, housekeeping.
Shared desktop and phone information order; illustrative numbers and English copy
below are localized in production. The hierarchy and fixed versus scrolling
regions are required; whitespace is not a pixel specification.

```text
Starting Kandev
Elapsed: 12 seconds

Reconciling queued work
Elapsed: 2 seconds
Progress unavailable

Completed steps
  Database backup                  3 seconds
  Prompt ordering                  1 second
  Service stores                   6 seconds
```

Phone keeps the title, total, and active step above one internally scrolling
timing list. Labels wrap above their durations when width requires it; there is
no horizontal table or second scroll owner. Respect dynamic viewport and safe
areas. Desktop keeps the existing compact dialog. Any existing phone controls
retain 44px targets. No new interactive control is required by the timing list.

UI-02: Transport interruption during startup retains UI-01 and adds the existing
localized `Last known` status. UI-03: A preparation failure never renders its
active operation as successfully completed; the existing failure/child-exit
surface owns error presentation. Both states retain known durations. On readiness
the page returns to the original requested route. No new completion screen.

## Tests

| Work order | Required new regression evidence | Acceptance coverage |
| --- | --- | --- |
| 01 | `startup_housekeeping_test.go`: `TestPromptSequenceReplaySkipsHistoricalAggregation`, `TestPromptSequenceReplayRepairsOutstandingState`; PostgreSQL counterparts | 002.1, 002.2 |
| 02 | Production composition barrier, cancelled/failed gate, remote isolation, preserved inventory handles | 001.1-001.5, 002.3-002.4 |
| 03 | One blocked restore, second session ready; focused selection; duplicate focus/Stop; unchanged prompt and queue-dispatch counts | 004.1-004.4 |
| 04 | Reporter transition/history, JSON compatibility, payload bound, locale completeness, bootstrap render, web parser/dialog | 003.1-003.5 |

The implemented regressions and passing evidence are recorded in the work-order
results. Concurrency and gating tests use deterministic channel barriers.

## E2E tests

- Chromium: extend `tests/layout/startup-page.spec.ts` and
  `restart-progress-startup-detail.spec.ts` for UI-01 through UI-03; add
  `tests/session/startup-housekeeping.spec.ts` for a real isolated restart with
  two candidate sessions and many stable stopped records.
- mobile-chrome: extend `tests/layout/mobile-startup-progress.spec.ts` with both
  renderers and boot reset; add `tests/session/mobile-startup-housekeeping.spec.ts`
  for selected-session recovery and unchanged prompt counts.
- Use current real backend restart fixtures and the Go startup-page fixture for
  renderer-specific states. Do not mutate the developer's instance. Existing
  agent-survival restart tests remain in the focused desktop/mobile run.

## Work orders

- [x] [Task 01: Restrict historical prompt repair](task-01-restrict-historical-repair.md)
- [x] [Task 02: Gate activation on local preparation](task-02-gate-runtime-activation.md)
- [x] [Task 03: Prioritize independent promptless restoration](task-03-schedule-session-restoration.md)
- [x] [Task 04: Show startup step durations and prove the combined flow](task-04-startup-timings.md)

Execute sequentially, in that order, in the primary session. Work orders share
lifecycle and reporter contracts and are not parallel-safe. No agent delegation
is authorized. Each work order lists its exact focused commands.

## Delivery

After all work orders and their stated checks pass, promote the paired draft
specifications, mark this plan implemented, and follow local `/commit`, `/push`,
and `/pr` skills. The request already authorizes those operations. Use the repo
PR template and documentation coverage preflight. Run `/pr-fixup` for actual CI
failures or valid review findings. Do not launch a PR-monitor subagent without
separate user authorization. Keep all parts in one branch and PR.

## Verification results

All four work orders are implemented in the primary session. Their results record
local evidence; PR CI and review handling remain part of delivery before merge.

- Race-enabled affected-package backend checks passed, including SQLite and real
  PostgreSQL prompt repair/replay and PostgreSQL store conformance. SQL guards passed.
- Scoped Go lint passed with zero issues, including the final plugin refinement.
  The updated-backend isolated restart rerun passed all three checks
  (two survival modes plus the populated two-conversation housekeeping scenario).
- All 120 focused frontend tests passed across 11 files; typecheck (8 GiB heap),
  targeted ESLint, Hant generation, i18n completeness, and new-copy ratchet passed.
- Chromium managed E2E: six checks passed; the final renderer capture rerun
  passed three checks. Mobile-chrome: three checks passed, including both
  renderers, two-conversation restart, and survival. Four compressed and inspected
  desktop/phone screenshots are ready for PR attachment.
- Public documentation: 63 tests and all 48 published pages passed. Spec document
  validation, 36 spec-linter tests, and all-file spec lint passed.
- Controlled populated repository measurement: first upgrade initialization
  462 ms, subsequent initialization 82 ms for 24 sessions and 6,000 messages under
  the race detector. This is local evidence, not a production latency promise.
- Work-order coverage preflight and final document validation passed against the
  complete changed-file set before PR publication.

## Risks

- Constructors currently start workers; a partial audit can leave an early
  producer racing preparation. Assert actual work is not admitted before the gate.
- Existing pending remote records must never be normalized as dead before adoption.
- Reordering alone does not reduce total cost; statement/query-plan proof and
  populated timing evidence must accompany the outstanding-state change.
- History expands the readiness payload. Test its maximum against bounded readers.
- Focus priority must not bypass session capacity or alter Stop/archival semantics.
- No wall-clock readiness target is promised without controlled measurements.

## PR remediation

PR #4609 review identified full-backlog metadata retention and stale completed
restart detail. The restore pass now drains each bounded inventory page before
reading another. Completed dialogs fetch the final ready snapshot before freezing
history. Activation also queues registrations made during an in-flight activation
so earlier failures cannot be bypassed. The new E2E helper waits on the persisted
agent message before asserting rendered content.

New regressions reproduced the queue, final-snapshot, and activation-order defects
before their fixes. Targeted Go race checks, frontend checks, and desktop/phone
completion checks are rerun before pushing; fresh captures replace the original
PR screenshots after the remediation commit. Exact-head CI and review evidence
remain external delivery work until the PR merges.

CodeRabbit's aggregate review also identified malformed terminal plugin inventory
blocking unrelated cleanup and stopped rows retaining active session state. The
fix skips invalid plugin metadata with a warning and classifies stopped rows
against one batched active-session read. Both regressions failed before their
fixes. Local ownership SQL reads intentionally remain required: an unavailable
remote endpoint is different from unreadable persisted ownership evidence.
Launcher readiness coverage now includes the largest registered fields, a full
active step, maximum numeric widths, and all 32 history entries.

Latest focused frontend run: 118 tests passed across 11 files. Typecheck with an
8 GiB heap and targeted ESLint passed. Affected-package race checks passed across lifecycle, orchestrator, startup,
backend composition and launcher. Four desktop and two phone rendered/restart
checks passed. Final Go lint reports zero issues; targeted ESLint and i18n ratchet pass.
Fresh post-commit captures precede the remediation push. The launcher upper-bound fixture is 7,349 bytes against a 32 KiB reader.

Review dispositions: the requested nonfatal remote-inventory read is not applied,
because this is local SQL ownership evidence required before launch admission,
not remote availability. The existing failure gate is now covered explicitly.
The alleged duplicate unpublished-turn reconciliation was a nil-service guard
followed by the sole call in both base and current code; once-only coverage passes.
The context-key policy suggestion remains optional: callers use the dedicated
`StartDeferredRemoteRecovery` method, with no mutable manager-wide policy. The
panic-only timeout-cancellation suggestion adds no live-process recovery benefit
because these workers have no panic recovery boundary; normal returns already
cancel both contexts. The combined phone capture test is retained to preserve its
two-asset manifest and existing scrolling assertions.

CI remediation: the existing restart-dialog test still required zero progress
polls after boot-id completion. Updated that stale assertion to cover final-ready
polling and explicit no-poll behavior for idle, starting and error phases. The
original assertion reproduced locally; the updated focused frontend run passed
129 tests across 12 files. CI's other frontend shard and lint/build job passed.
This correction changes tests and delivery records only; rendered captures remain
valid for the unchanged production tree.

Kubernetes CI remediation: the prior head finished with 86 passing checks; its
only test failure was a stopped-session reconciliation log wait, with the E2E
aggregate failing as a consequence. The unchanged test reproduced that failure
in an isolated real Kind cluster. Replaced the obsolete implementation-log wait
with retained session state, user-message identity, Pod UID and PVC UID assertions
before the existing workspace-readiness and manual-recovery flow. Both cases
passed locally with retries disabled. This changes tests and delivery records;
production behavior and the verified screenshots remain unchanged.
