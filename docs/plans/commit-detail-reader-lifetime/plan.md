---
created: 2026-10-03
status: implemented
requirements:
  - REQ-UI-PR-ONLY-COMMIT-DETAILS-001
system_design:
  - ../../specs/ui/system-design/commit-detail-target-types.md
legacy_specs: []
---

# Implementation Plan: Commit-detail reader lifetime

## Overview

Retire request publication and retry admission when a live commit reader closes
or its committed fetch target is replaced. One sequential work order delivers
faithful regressions and the minimal hook repair after the explicit
reviewed-root implementation release. Task 01 is complete.

UI owns the existing reusable cross-source commit-reader contract. Integrations
retains provider credentials, authorization, and service transport; this repair
does not change those boundaries or historical commit navigation.

## Evidence and root cause

`useCommitDetail` increments `requestSeqRef` when fetching, but never invalidates
it on unmount or committed target replacement. Its latest pending rejection
can reach the surviving real `ToastProvider`, which also schedules frontend
error reporting. Retained callbacks can admit transport after their reader has
retired. Waiting for a new passive fetch is insufficient at the commit boundary.

Accepted parent evidence: `/tmp/kandev-commit-detail-lifetime-repro.test.tsx`,
inspected read-only. The real React Reader uses the production hook, real
`StateProvider`/`createAppStore`, and real `ToastProvider`. Only detail transport
and the frontend-error-report sink are mocked, with the protocol class retained.
After committed Reader removal, deferred rejection leaves an erroneous toast
`Request failedretired detail failure` instead of no toast. Parent's joined
handle 7716 exited 1: one expected correctness failure and one current-reader
positive PASS (45 ms failing test, 51 ms tests, 2.97 s package). The temporary
in-repo source was removed. Do not replay this proof in the design phase.

## Scope

### In scope

- Existing hook's instance/committed fetch-input lifetime and latest-request
  guards, including retained retry admission and layout-bound retirement.
- Permanent real-provider DOM regressions and current source/error/readiness
  controls in `use-commit-detail.lifecycle.test.tsx`.
- Requirement .9, existing source contract .1-.3/.6/.8, design, and this delivery
  record, including exact verification results after implementation.

### Out of scope

- `useCommitDiff`: current production search found only its definition.
- Transport abort, global coordinators, store/reporting policy changes,
  ToastProvider timer cleanup, local previews, parsers, backend work, new
  provider contracts, identity redesign, optional polish, or unrelated tests.
- Layout/copy/touch/navigation/scroll/breakpoint changes, browser, build, E2E,
  broad Vitest/Go/audit, parent-proof replay, or native/persistent delegation.

## Technical approach

Extend only `hooks/domains/session/use-commit-detail.ts` with a small lifetime
following `hooks/use-file-upload.ts`. Layout setup/cleanup governs admission;
the callback captures its owner, and request sequence guards every completion
branch. Never mutate current-owner refs during render. StrictMode replay must
admit the current reader without allowing pre-cleanup work to become current.
Use the existing target key, request routing/dependencies, and return shape.

| Source / consumer | Transport and identity | Required preservation | Evidence |
| --- | --- | --- | --- |
| Local rows/panel/phone | Existing detail transport; repo/SHA plus actual session/task/readiness inputs | Local payload and readiness retry; retire replaced fetch context | Real-store local-context controls |
| GitHub rows/panel/phone | Workspace/owner/repo/SHA | No local routing/fallback; local readiness must not refetch | Remote payload/error controls |
| All live readers | Per-instance lifetime and request sequence | Current success/error/retry; stale publication and retained callback denied | Deferred real-provider lifecycle tests |

No new fallback is added for unsupported response shapes: current protocol
failures keep their localized error and toast while owned.

## Mobile and rendering assessment

State-only frontend exception: `CommitRowFiles` and `commit-detail-panel.tsx`
use the shared hook. Phone `MobileDiffSheet` embeds `CommitDiffView`; its existing
full-height sheet, fixed close header, internal scroll owner, safe area, and
touch controls are unaffected. Desktop remains a dockview panel and inline
Changes list. No new composition or ASCII geometry preview is needed. The
observable correction is that a closed/replaced reader contributes no stale
toast or report; current readers retain the existing error and retry UI.

## Tests

In `hooks/domains/session/use-commit-detail.lifecycle.test.tsx`, use actual
providers and transport-only mocks. Map .9 to closed/reopened success/failure,
committed target replacement (including a layout-bound old-callback attempt),
retained retry denial, independent instances, StrictMode replay, and latest
request overlap with pending/loading checks. Map .1-.3/.6/.8 to current local
and GitHub routing, local readiness retry, unrelated GitHub readiness changes,
protocol failure, current error toast/report, and successful on-demand retry.

Use the permanent close/rejection regression as meaningful RED after release.
Measure DOM state, toast DOM, report sink, and actual admissions rather than
mirroring implementation refs. Clean owned timers and deferred work. Real
provider rendering gives end-to-end evidence for this state-only boundary;
no Playwright test or viewport-dependent behavior is involved.

## Work orders

- [x] [Task 01: Retire commit-detail readers](task-01-retire-readers.md)

## Verification results

Implementation is complete after the later reviewed-root release. Task 01 is
done; the owning requirement is active and the design is current. Permanent
real-provider RED recorded 1 expected failure / 1 positive PASS; final GREEN
recorded 18 passing lifecycle/source/readiness/protocol/retry controls (111 ms,
2.82 s package). Changed lint, typecheck, i18n check/ratchet, catalog/spec lint,
actual coverage, and diff checks passed. Full command and joined-handle receipts
are in [Task 01 results](task-01-retire-readers.md#results). Hosted CI, exact-head
semantic review, actual merge, and cleanup remain external delivery gates.

Design checkpoint, 2026-10-03:

- `python3 scripts/list-docs.py validate`: exit 0; 343 decisions and 1325 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: exit 0; all specifications passed.
- Repository `validateCoverage` preflight: exit 0. Actual four-document diff
  is documentation-exempt; projected hook change is covered by this ONE work
  order, all referenced ACs exist, and its design is declared by the manifest.
- `git diff --check` and package status inspection: exit 0; four documentation
  paths only, all unstaged/uncommitted. One manifest and one pending work order.
- No install, production change, permanent test change, proof replay, runtime,
  browser, or heavy check. No running process handles were created.

## Risks

- StrictMode replay can re-admit an old request if cleanup only flips a flag.
- A retained callback can increment the shared sequence or dispatch before a
  publication guard; admission must precede both.
- Commit-boundary cleanup must not depend on passive effect timing.
- Local fetch dependencies must retain readiness retry without coupling GitHub
  requests to unrelated session readiness. Do not invent new loaded-data keys.

## Execution and completion barriers

No production or permanent tests before the later explicit reviewed-root
implementation interrupt in session `0be54e5e-a656-4c04-826d-bf52d88bca84`.
One local heavy operation at a time. Join every returned process handle before
the next install/test/lint/hook. Only one pinned frozen install if absent;
preserve lockfiles, caches, foreign processes, worktree, and parent proof.

Normal hooks, conventional commit, push, and ready PR are authorized after
release. Freeze the head absent a valid correction; no moving-main rebase,
synthetic tests, bypass, or gate weakening. Join one owned `scripts/pr-await`
before any replacement. Completion requires terminal required hosted checks,
authenticated CodeRabbit App 347564 full substantive all-file review of the
exact current head and disposition of every finding, independently verified
actual MERGED SHA/tree/blobs/remote, and joined owned cleanup. Send parent a
queued handoff now and a queued verified completion receipt only after those
gates. No additional workers, tasks, or session tabs.
