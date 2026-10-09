---
created: 2026-10-03
status: done
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
  - REQ-UI-WORKSPACE-FILE-TRANSFER-003
  - REQ-UI-WORKSPACE-FILE-TRANSFER-004
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
legacy_specs: []
---

# Implementation Plan: Upload Owner Lifetime

## Overview

Retire unfinished workspace upload batches when their UI owner unmounts or changes session,
settle callers, and prevent later file requests and stale UI reports. Preserve confirmed writes
from requests already in flight. One sequential work order owns the hook and immediate caller
evidence; implementation was authorized by the parent's later explicit release in this same session.

## Evidence and root cause

Inspected base: `2d330671197bb06cf4a634e4303ceeb3114113f4`. The parent independently exercised the
production hook with deferred/mock transport in the read-only archive
`/tmp/kandev-upload-unmount-repro.test.ts`: a parked promise remained unsettled after owner
unmount, and releasing the held first upload after unmount issued two file requests where one
was expected. Both selected tests failed; parent handle 30188 was joined with exit 1. Accept
this evidence without rerunning the temporary harness; write focused permanent regressions.

`useSessionChangeReset` clears batch refs on session change but has no unmount cleanup. The
upload loop's request guard therefore remains true after disposal. Preflight rejection and
post-upload item patches lack a lifetime check. `resolveConflicts` consumes the parked ref
before awaiting uploads, so retirement must also cover that executing path. The immediate
entry point calls `report(await uploadFiles(...))`; cancellation already suppresses reporting,
but rendered evidence must establish suppression at the actual caller continuation.

## Scope

### In scope

- Existing [requirement](../../specs/ui/requirements/workspace-file-transfer.md) and
  [design](../../specs/ui/system-design/workspace-file-transfer.md#upload-owner-lifetime),
  principally `AC-UI-WORKSPACE-FILE-TRANSFER-003.7` through `003.10`.
- Instance-local lifetime and batch ownership in `apps/web/hooks/use-file-upload.ts`, faithful
  targeted hook regressions, and rendered tests of the real upload entry point.
- `apps/web/components/task/use-file-upload-entry-points.tsx` glue only if the rendered
  transport/continuation evidence establishes a reporting gap after the hook fix.

### Out of scope

Backend, transport/API changes, AbortController frameworks, rollback, generic coordinators,
new retry or request ordering policies, layout, copy, touch sizing, navigation, feature flags,
runtime/harness routing, other files, and extra work orders. Preserve foreign changes,
processes, worktrees, and shared caches. The original workspace-file-transfer package records
its delivered feature work; this repair neither reopens nor rewrites its historical results.

## Technical approach

Replace session-only reset with a bounded instance-local lifecycle that invalidates batch
ownership on disposal/session cleanup and becomes live again on setup. Capture stable lifetime
identity in callbacks and async work; guard every request, conflict parking, state patch, and
owner finalizer. A same-session return must not revive an old lifetime. Keep batch identity
monotonic. Manual cancellation remains scoped to a parked batch.

Retire parked promises exactly once during cleanup. Awaiting transport completes naturally;
on settlement return cancellation while retaining accumulated successful writes/failures and
skipped paths. Guard preflight failure as well as success. Active uploads started from conflict
resolution retain their original caller resolver until completion. Cleanup does not perform
unmount state updates or promise abort/rollback.

Render a small owner that mounts the actual entry-point hook, hidden inputs, and conflict
dialog; mock transport and toast delivery, not `useFileUpload`. Prove cancelled outcome routing,
normal completion reporting, and unmount/session replacement suppression. Only add local
entry-point lifetime guarding if a real reporting regression demonstrates the need.

## Tests

Permanent test names below describe required scenarios, not internal implementation predicates.

| Criteria | File and scenarios |
| --- | --- |
| `003.7`, `003.8`, `004.1`, `004.5` | `hooks/use-file-upload.test.ts`: `settles a parked batch on owner unmount without uploading any selected file`; include both conflicting and unconflicted entries and skipped evidence |
| `003.7`, `003.8`, `003.10` | Same file: `retires deferred preflight responses after unmount or session replacement`; exercise resolve with/without conflicts and reject, with no subsequent write/conflict publication and current session unchanged |
| `003.7`, `003.8`, `003.9` | Same file: `stops direct and conflict-resolved uploads after owner retirement`; hold first file, retire, release success/failure, assert one request, cancelled caller, accurate evidence, and no replacement-state contamination |
| `003.10` | Same file: `rejects retained callbacks from a retired lifetime`; `keeps independent upload owners and replacement sessions live`; `uploads after StrictMode setup cleanup setup` |
| `001.5`, `003.7`, `003.9` | `components/task/use-file-upload-entry-points.test.tsx`: `suppresses retired upload reports while current uploads still report`; render real inputs/dialog/hook with deferred transport; cover direct and conflict-resolved completion, failure, disposal and session replacement |
| `001.5`, `004.3` through `004.6`, `003.9` | Existing hook success/failure/skipped/session-change/per-file resolution tests, plus `preserves empty and all-skipped outcomes`; rendered normal success/partial failure/manual cancellation controls |

Join every caller promise and release every test-owned transport. Do not weaken assertion
timeouts, retry failures, or test only guard booleans. Expand a scenario only to cover a
distinct causal path. Exact affected suites replace a full local audit.

## Mobile and rendered verification

The shared lifecycle fix changes only state/result delivery inside existing owners. It changes
no rendered composition, layout, touch behavior, scroll owner, navigation, or breakpoint logic.
Use the mobile-parity state/data exception with targeted hook and rendered component tests.
No ASCII layout preview or new mobile Playwright scenario is needed. Run no browser/build/E2E
suite absent concrete new evidence; this package's end-to-end UI evidence is the real mounted
entry point through input/dialog, hook, mocked transport, and toast boundary.

## Work orders

- [x] [Task 01: Retire upload batches with their owner](task-01-retire-upload-batches.md)

One work order, sequential, no dependencies and no agents/workers/tasks/sessions. The design
handoff ended before implementation; the parent subsequently released execution in this same
session. Implementation is done; external PR/merge gates remain in the MCP task plan.

## Verification strategy and resources

Run the exact [work-order commands](task-01-retire-upload-batches.md#verification) sequentially.
One heavy local command at a time, Vitest one worker, Node heap capped at 4096 MiB. If workspace
dependencies are absent, install once with the frozen lockfile from `apps/` before testing.
Retain and join every owned process handle; never infer completion from a replacement run.

Documentation coverage must use the actual changed/untracked package files and owning refs,
without staging or committing the design package. After implementation record actual test
counts and command outcomes, run normal hooks, commit/push, and publish a ready PR promptly.

Delivery uses one owned `scripts/pr-await` monitor, retained and joined before replacement.
Require hosted gates terminal clean and authenticated configured current-head full all-file
semantic review with all findings dispositioned; CodeRabbit app 347564 full report suffices.
Freeze the published SHA absent a corrective finding. No ACK/skipped substitute, duplicate
full review request, optional second wait, or rebase for moving main. Merge normally with the
expected head and no admin/bypass, then independently verify actual MERGED SHA, content and
remote state. Completion requires all owned handles joined and clean owned changes; dependency
directories may remain for parent archive.

## Verification results

Task 01 implementation is done; see its [results](task-01-retire-upload-batches.md#results).

- One frozen dependency install using existing pnpm 9.15.9: passed; lockfile unchanged.
- Permanent production-hook RED and real rendered completion-to-report RED observed before
  their respective production changes. All owned RED handles joined.
- Exact affected hook/entry-point/conflict-dialog suites: 54 passed after the commit-boundary
  correction (initial owner-disposal implementation: 52 passed). Final affected rendered
  suite after a test-only type correction: 12 passed. All test handles joined.
- Changed-file eslint, typecheck, i18n check and ratchet: passed, one heavy command at a time
  with one Vitest worker and the 4096 MiB Node cap. All check handles joined.
- Catalog: 343 decisions and 1319 specifications validated. Spec lint and diff check passed.
- Actual diff documentation coverage: covered/ok, one work order, no errors.
- No public docs change needed: internal owner-lifetime correction changes no public API,
  command, label, navigation, or screenshot. Approved mobile-parity state/data exception used.

Node 24.18.0 and existing project-pinned pnpm 9.15.9 run through `mise exec` from non-login
Bash; login zsh strips the Node PATH. Both `mise.toml` and `apps/package.json` pin pnpm 9.15.9.
No agents/workers/tasks/sessions, full local suites, builds, browser tests or foreign-resource
mutations were used. Normal hooks, ready PR publication, hosted gates, full configured semantic
review, expected-head squash, actual merged-content verification and joined cleanup remain
external delivery gates in the MCP task plan. Implementation completion is not task completion.

## Risks

- A file already dispatched can succeed after retirement; retaining its returned path is required.
  A transport that never settles can keep an awaiting caller pending; transport timeout/abort
  behavior remains outside this correction.
- Resolving a parked caller too early during active uploads would erase in-flight write evidence.
- Setup replay, same-session returns, stale callbacks and finalizers must not revive old batches
  or clear a replacement batch.
- If hosted CI fails, extract the exact leaf/log and notify parent queued without blind retries.
  Recurrence of `mobile-changes-history-regression.spec.ts:85` (expected 2x4, actual 20x4) needs
  parent consultation; historical same-head retry passes are not current-head evidence. Do not
  run local geometry builds, broad audits, or speculative unrelated fixes.

## Parent communication

Task `dc71938c-0e50-4deb-9717-8794d496cb97`, session
`cd2ab561-1a60-488d-8fc1-006958e95d01`; parent
`14825981-b175-411d-999a-31ddc2aa5fc3`. Send queued parent notifications at design handoff,
PR publication, concrete blockers/recovery, and verified merge plus joined cleanup. Parent
instructions use interrupt delivery. Preserve the task-plan system marker, user edits, question
barriers and title ownership. A critical unsafe unresolved choice uses the parent question tool
and ends the turn; routine choices proceed under standing autopilot. Parent retains its proof
archive read-only until merge and owns archive/next-child release.

Implementation explicitly released by parent on 2026-10-03 in this same session. Task 01 implementation is
done. Use existing installed pnpm 9.15.9 via mise exec, with no packageManager/lockfile
change. The design checkpoint remains historical; production work is now authorized.

PR review found commit-time retirement must precede passive effects. Task 01 reopened only for
that causal correction. Real React concurrent-commit transport REDs assert actual request
counts (0 vs 1 after preflight retirement, 1 vs 2 after active retirement). The fix uses the
existing local lifecycle and reporting effects at layout time; no new API or framework.

Commit-boundary correction completed: two faithful additional REDs became GREEN, all 54
exact affected tests passed, changed lint/typecheck/i18n/docs/actual coverage passed. Task 01
is done again; published-head update is justified only by the actual review finding. No main
rebase or extra local suites occurred. Hosted/review/merge gates remain in the MCP task plan.
