---
created: 2026-10-06
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-BROWSE-STATUS-001
system_design:
  - ../../specs/integrations/system-design/github-browse-status-batches.md
legacy_specs: []
---

# Implementation Plan: GitHub PR Status Batch Scope

## Overview

Make browse status results readable only for their current requested workspace
and existing content batch. One sequential work order adds meaningful real-hook
and real-list regressions, corrects the hook's local result state, and verifies
the affected suites. ROOT reviewed the full four-file design and hashes before
granting implementation in this same primary session. Local results below are
independent of hosted review and merge, which remain pending at publication.

## Baseline and accepted evidence

Accepted ROOT proof ran at 2026-10-06T13:54:09-15Z against
`f66293552d15d53d1ad20b114a9d0c223722c04e`. Three causal failures demonstrated:
completed A summaries visible while B was pending for the same PR; previous
batch summaries visible after membership changed; actual newly scoped `PRList`
showing old green `8/8` badges. Equal-content reuse and actual current ACK were
the two passing controls. This was not a setup, transport or resource failure.

Original native session `40230`, chunks `db960a`/`47d771`, actually joined exit 1
in 5.857s; original processes/groups were gone. The protected mode-0400 archive
`/tmp/kandev-github-pr-status-batch-candidate.test.tsx` has SHA256
`aad38d9254c4b19fa9496d5c9a70f077fad7d50b9e87825c85c6d2a12bdf2fc4`.
Full receipts and log remain in the task's external plan and original
`/tmp/kandev-root-pr-status-batch-proof-{receipt,classification,native}.json`
and `.log`. Do not replay, copy, mutate or remove them.

Design checkout HEAD is `1c4d5b4aa85af8857befbd6056eeaeecb409b704`.
Current hook blob `e51e51861439e5fc4935959364f46520e60555a2` and list blob
`af3096dcc0256d7739f71b061c2177498472c30a` match the proof exactly; targeted
baseline-to-current diff was empty. No rebase or product execution was needed.

Confirmed cause: the hook's result Map has no producer batch identity and is
returned unconditionally. Cancellation guards future publication but does not
fence the retained completed result on a new request's first render.

## Scope

### In scope

- Existing `usePRStatuses` local snapshot and immediate return eligibility.
- New permanent real-hook/transport/list regressions under the later author grant.
- Existing hook/key, list launch and badge controls; exact affected checks.
- Truthful lifecycle/results updates to these four design files.

### Out of scope

- Request counters, shared coordinators, new frameworks and global mutable maps.
- Backend/client transport, provider cache, scheduling, retries, polling, HEAD or
  updated-time refresh, permissions and persistence.
- Reverse `usePRKeyToTasks`, task-linked status, other providers and issue rows.
- Production consumers without new causal evidence and ROOT review.
- UI layout/copy/navigation/touch/scroll/breakpoints, package or test config.
- Browser/build/E2E and public guide edits absent actual need.
- Rebase or unrelated changes; the completed design phase authorized no code or publication.

## Technical approach

Implement the [owning design](../../specs/integrations/system-design/github-browse-status-batches.md)
in `apps/web/components/github/my-github/use-pr-statuses.ts` only. Stamp the Map
with its successful producer key, return it only for a matching nonempty current
key, and return an instance-local empty Map otherwise. Preserve `completedKey`,
effect dependencies and cancellation. In particular, retained A reuse while B
is pending remains possible; failed B can still clear A without guaranteeing
another A request. A key stamp does not replace the cancelled-read guard.

| Provider and transport | Identity | Coverage and fallback |
| --- | --- | --- |
| GitHub existing `getPRStatusesBatch` | Requested workspace plus ordered owner/repo/number list | Matching completed Map; mismatch/null/empty gives empty Map; missing statuses gives current empty result |
| GitLab, plugins, issues | Separate adapters/contracts | Unchanged; no cross-provider claim |

## Observable row outcome

The existing desktop/phone row keeps its current title, metadata and task action.
After scope changes, status badges from another batch disappear immediately;
current acknowledged badges appear through the same row. No composition preview
is required for this pure state correction: no control order, layout, copy,
navigation, touch, scrolling or breakpoint behavior changes.

## Tests

Requirement `REQ-INTEGRATIONS-GITHUB-BROWSE-STATUS-001`, AC .1-.7 maps to the
[work-order regression matrix](task-01-scope-status-results.md#regression-matrix).
New file: `components/github/my-github/use-pr-statuses.scope.test.tsx`.
Real `PRList` integration lives there. Keep existing
`use-pr-statuses.test.ts`, `pr-list.test.tsx` and `pr-status-badges.test.tsx`
controls in affected GREEN. Only the batch API transport is partially mocked.

One causal RED run against unchanged production includes pending workspace,
membership and first-list-commit regressions plus a current-ACK positive control.
Permanent tests must be authored independently; the accepted proof is evidence,
not a scratch test to copy or rerun.

## Mobile and public documentation assessment

The shared list/badges use the same hook result on desktop and phone. Apply the
mobile-parity pure-state exception with targeted hook/component tests. No browser,
build or E2E run is planned because there is no viewport-dependent interaction.

The public Integrations guide's workspace browsing and phone controls remain
accurate. This package corrects which local batch may supply existing badges;
it adds no command, setting, API shape, label, navigation or operational advice.
Internal docs only; no public guide, root README or screenshot catalog edit.

## Work orders

- [x] [Task 01: Scope status results](task-01-scope-status-results.md)

One wave, one sequential work order, same primary. No delegation or new sessions.

## Verification results

Design documentation checks passed on 2026-10-06: catalog validated 355 decisions
and 1,404 specifications and discovered both new owning specs; all 36 specification
linter tests passed; full specification lint passed. Actual four-file changes
were documentation-exempt with `errors: []`; the separately labelled planned-hook
reference preflight was covered with `errors: []`. This is reference validation,
not an actual production change or test run. Final whitespace, full-file inventory
and process receipt review are recorded in the external task plan.

Permanent regressions and the sole production hook correction are implemented.
The causal RED selection produced five intended failures with its current-ACK
control passing; affected GREEN passed four suites and 36 tests (26 new
regressions and 10 existing controls). Changed ESLint, normal project typecheck
including pretypecheck generation, and actual changed-source i18n ratchet passed.
Implementation documentation gates are recorded in the work order. Publication,
hosted review and merge remain pending at this local checkpoint.
The work order defines exact bounded RED/GREEN, changed ESLint, normal project
typecheck, actual-path i18n ratchet and documentation-reference checks for the
implementation grant. Do not infer success from the accepted baseline.

## Risks and checkpoints

An effect-only reset can hide the bug after testing-library flushes effects;
record the first render/commit. Completed-key reuse must not be mistaken for
freshness or navigation-generation behavior. Keep existing failure semantics
and distinguish requested fixture rejection from real transport/setup failure.

ROOT full-file/hash review and the later same-primary implementation interrupt
were satisfied. The exclusive GLOBAL LOCAL-HEAVY grant covers local delivery;
MERGE remains NONE. Each install/test/lint/
typecheck/heavy hook requires the ONE GLOBAL LOCAL HEAVY lease and serial original
handle joins with process-group-gone evidence. Resource/timeout/transport/unknown
or out-of-scope results checkpoint ROOT without automatic retry.

The external task plan preserves standing normal publication, one frozen-head
hosted collector, full current-head configured reviews, exact failed-job retry
limits, separate ROOT merge authorization, actual merge verification and return.
Finalize local done/history before publication; hosted review and merge remain
pending. Design completion is END DESIGN / WAITING, not implementation authority.
