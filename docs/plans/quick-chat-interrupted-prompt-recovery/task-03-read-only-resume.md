---
id: "03-read-only-resume"
title: "Resume from a read-only workspace"
status: done
wave: 3
depends_on:
  - "02-manual-resume"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.11
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Resume from a read-only workspace

## Summary and scope

A restored workspace can show a passive notice without a stopped-agent composer card.
Expose the existing explicit Resume action in that notice and honor its busy state.
After successful native resume, acknowledge the current journal's interrupted-unknown
submissions before resolving the SQL recovery block. Retain unknown history and forbid
replay. Refuse live work, ownership mismatch, and unavailable durable storage.

## ASCII UI preview

Desktop and mobile use the existing shared recovery notice:

```text
+-----------------------------------------+
| Workspace restored in read-only mode     |
| > Recovery details                      |
| [Resume session]                        |
+-----------------------------------------+
```

The action remains visible with details collapsed. Busy disables it. When the
composer owns recovery, it remains the sole owner of the Resume action.

## Acceptance

1. Read-only fallback with no composer recovery owner exposes an actionable Resume.
2. Successful explicit native resume acknowledges only interrupted-unknown journal
   work from the trusted owner, preserves its payload/outcome, and admits a new prompt.
3. Failed acknowledgement leaves the SQL block open. Prepared, accepted, dispatching,
   and completed work missing terminal evidence cannot be acknowledged by native resume.
4. Desktop and phone render a reachable Resume action and send a distinct follow-up.

## Verification

Run journal, process, lifecycle, and orchestrator regression tests with trimpath and
race detection; focused recovery UI/hook tests; frontend typecheck and ESLint;
managed desktop/mobile browser recovery tests with screenshots; documentation checks.

## Dependencies and risks

Work order 02. No implicit resume grant on focus or workspace restoration; no native
identity replacement or harness generation rotation. Use existing authenticated retirement
transport and preserve existing continuation recovery semantics.

## Parallelism

`sequential`

## Results

- The read-only UI regression failed because no Resume action was rendered; it passes with
  the existing callback and busy-state guard. Composer ownership still suppresses duplicate actions.
- Journal, process, lifecycle and orchestrator regressions failed before their respective fixes.
  Real bbolt tests retain unknown payload/outcome, prevent replay, admit a distinct instruction,
  reject live and foreign work, and survive journal reopen and authenticated backend adoption.
- Orchestrator tests retain SQL blocks on launch or journal failure and preserve Office's
  existing scheduler recovery gate. Managed Go cache recovery retains its startup failure behavior.
- Focused frontend recovery checks: 7 files, 114 tests passed. Typecheck and zero-warning ESLint passed.
- Desktop and phone managed Playwright recovery flows passed: read-only fallback without a
  composer owner, visible Resume with collapsed details, native identity retained, no replay,
  and a distinct follow-up. Repository-free Quick Chat flows also passed on both viewports.
- Desktop and phone screenshots were captured with synthetic data, inspected and compressed.
- Documentation catalog/specification checks and all 36 specification-linter tests passed.
- The broader process suite has two existing fixture failures: TestProcessRunnerCapturesOutput
  and TestProcessRunnerStopLogsSignalAttempts. Both also fail on an isolated base 422a63cab.
  Final focused race checks and Go lint results are recorded in the PR validation.
- Public how-to documentation now explains the read-only notice's existing Resume action.
- PR publication is tracked in the task handoff; no production recovery data was modified.

## PR review remediation

- Merged the current main branch into the PR branch without conflicts.
- Validate every unresolved SQL row against the peer by submission ID, session,
  incarnation, generation, and payload hash before any retirement. Missing SQL access,
  live SQL work, missing journal records, and mismatches fail closed. After retirement,
  re-read both stores and require matching retained acknowledgement before clearing recovery.
- Lifecycle regressions reproduced missing and mismatched SQL evidence being accepted.
  They now reject it without mutating either store and prove matching acknowledgement
  remains adoptable after restart, including earlier generations and idempotent recovery.
- Browser fixtures seed the same interrupted record into SQL and the real retained bbolt
  journal using the mock binary. Both desktop and phone flows inspect retirement before
  sending a new instruction; read-only workspace recovery also restarts the backend first.
- The PostgreSQL activity-retention test now waits for startup cleanup to finish before
  sending its tick and joins its goroutines on assertion failure. The existing test passed
  15 PostgreSQL 16 race-enabled repetitions before remediation; the complete retention
  suite passed 30 repetitions afterward. The CI failure exposed a startup/tick race.
- Focused verification commands: `go test -trimpath -race` with native resume,
  durable adoption, journal-fixture, and orchestrator recovery cases; frontend typecheck
  and zero-warning ESLint; managed Chromium and mobile Chrome recovery specs with
  retries disabled; Go lint on the three affected packages against main; documentation
  catalog validation and full specification lint.
- Remote CI and review verification remain pending until the remediation commit is pushed
  and the new head's checks and review threads are clear.
