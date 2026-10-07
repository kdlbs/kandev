---
created: 2026-10-06
status: implemented
requirements:
  - REQ-COORDINATOR-COORDINATORS-004
system_design:
  - ../../specs/coordinator/system-design/coordinators.md
legacy_specs: []
---

# Implementation Plan: Coordinator list read publication

## Overview

Keep coordinator list completions bound to the current read and hook lifetime.
One sequential order added faithful regression coverage and the local hook
correction after ROOT's later explicit implementation release. The initial
design handoff and its failed Node setup receipt remain recorded below.

Coordinator owns workspace coordinator identity and its settings list. Reuse
[the requirement](../../specs/coordinator/requirements/coordinators.md) and
[design](../../specs/coordinator/system-design/coordinators.md#settings-list-read-publication).
Only `AC-COORDINATOR-COORDINATORS-004.8` is added. The earlier
[workspace-coordinator package](../workspace-coordinator/plan.md) and its task 02
remain broader feature records; this order does not claim their unfinished work.
The existing core-coordinator ADR needs no change for this local correction.

## Evidence and scope

Creation HEAD: `05c41b11e830a9861534200949c3bbac473b57f7`. The accepted ROOT
archive `/tmp/kandev-root-coordinator-list-next-candidate.test.tsx` has SHA256
`7f92142e51b13c2cb9a622545460fd6e635f983828c0041b50a590769a8af55a`.
Classification: `/tmp/kandev-root-coordinator-list-proof-classification.json`;
receipt/log: `/tmp/kandev-root-coordinator-list-publication-proof-receipt.json`
and `/tmp/kandev-root-coordinator-list-publication-proof.log`. Inspect read-only;
never replay, copy, import or mutate these proofs. ROOT original session `45481`,
chunks `c2e481`/`3f0f2a`, joined exit 1: three causal failures and one passing
positive control. Receipt records removed scratch and all original processes
gone. This package records accepted evidence, not a new reproduction.

In scope: local list request ownership/lifetime, real-hook/store and rendered
list transport-only regression tests, existing cached reads and error/retry.
Out of scope: generic framework, backend/API, feature rollout, mutation ordering,
whole-store redesign, markup/copy/layout, browser/build/E2E, main-only rebase,
synthetic tests, administrative bypass and optional polish. Other children are
active: do not revert their edits or touch their worktrees, proofs or caches.

## Technical approach

`useCoordinators` currently compares only workspace strings. Its already-loaded
branch leaves B's in-flight marker live after A-loaded -> B-pending -> A, and
overlapping refreshes share the same marker. Initial loads and refreshes now
share local read tokens. Layout cleanup retires their memoized lifetime at
commit before passive effects; retained callbacks refuse stale admission. Rows
are gated by accepted workspace/store-action identity before passive loading.
The existing slice and CRUD callbacks remain. Audit found only `CoordinatorsListPage` and
`CoordinatorAddPage`; the latter consumes `create` only.

## Tests

All new causal cases cover `AC-COORDINATOR-COORDINATORS-004.8`. Author new tests
independently in the two files named in task 01, using describe prefix
`Coordinator list publication`. Use real `StateProvider`, `useAppStoreApi`,
`useCoordinators`, `listCoordinators` and `CoordinatorsListPage`. Partially replace
only API client `fetchJson`; defer resolve/reject, discriminate real endpoint
paths and let unrelated expected settings reads return typed fixtures.
No hook/store/page/API-domain mocks or proof archive imports in causal suites.

| Scenario | Required evidence |
| --- | --- |
| A-loaded -> B-pending -> return A | A rows/loaded state survive late B success; no B name/Open/Configure links under A URLs; cached return adds no needless A request |
| Accepted A -> B commit before passive load | Layout observer sees no A link under B URL; B-current subsequently renders correct links |
| Older/newer refresh, both completion orders | Only newest read publishes; older success/failure/finally cannot replace rows, set/clear error or end newer loading, both while newer is pending and after it settles |
| Initial load overlapped by refresh | Same publication rule applies across both read entry points |
| Current failure and retry | Initial failure is error, not loaded-empty; real Retry starts a read; repeated failure stays retryable; success clears error and accepts rows; refresh failure retains accepted rows |
| Identity/lifetime changes | A/B/A while pending, A -> null -> A, unmount/remount and fresh real-provider owner for the same workspace; obsolete callbacks cannot affect either owner's accepted state |
| Positive controls | A-pending -> B-current ignores A; current success/empty list works; same-workspace cached rerender does not refetch; null has no request; StrictMode cleanup allows current replacement to finish |

RED must fail for the proved causal assertions, with the distinct-workspace
control passing. Setup, resource, timeout, unknown or out-of-scope failures are
not RED. GREEN reruns the same newly authored cases and required compatibility
checks. Pending promises must be resolved/rejected and awaited during cleanup.

## Mobile and E2E assessment

The existing list route and card grid use the same hook on phone and desktop.
This package changes list-state publication only. The mobile-parity state-only
exception applies; real rendered list/Retry/link assertions prove the shared
outcome. No UI preview, mobile composition change, Playwright, browser or build
is required. If investigation shows viewport or routing mechanics are causal,
end work and report to ROOT before expanding this scope.

## Work orders

- [x] [Task 01: Correct coordinator list read lifetime](task-01-list-read-lifetime.md)

Sequential, no dependencies or delegation. No new task/session or model switch.

## Verification and delivery

Design: cheap catalog/spec lint, PR documentation coverage and diff checks only;
GLOBAL HEAVY NONE. Implementation: task 01 owns exact bounded causal and
compatibility Vitest, changed-file lint, project typecheck, i18n, docs coverage
and normal commit hooks. One pinned pnpm 9.15.9 frozen apps install if absent.
Keep every original handle, cutoff, log and PID/PGID; join and prove gone before
dependent work. No automatic replay, cache wipe or foreign kill. Routine
in-scope fixture/lint corrections rerun affected checks only.

After later authorized implementation, normal ready PR delivery freezes the SHA
except grounded findings. Use ONE retained original 45-minute all-terminal
collector, configured App `347564` substantive FULL current-head ALL-files
review (automatic evidence if sufficient; one necessary real-gap request only),
required six checks plus actual Backend/Frontend/E2E parent checks. Preserve the
original collector handle and join it before dependent work. MERGE NONE until
ROOT's separate serial grant. ROOT watches this primary directly; no completion
gate depends on queued callbacks.

## Verification results

At the original design handoff, no product commands, installation, production
changes or permanent tests ran. Design catalog validation, specification-linter tests
and full specification lint passed. Earlier `git diff --check` passed; four
artifacts were confirmed unstaged/uncommitted. PR document coverage did not run:
the original evaluator spawn failed with `FileNotFoundError: node`. Per ROOT's
setup-failure rule, no runtime installation, fallback or replay followed; the
remaining supervised diff check was not reached.

Original docs supervisor session `60788`, initial/joined chunks
`360598`/`62944a`, joined exit 1. Started check PIDs/PGIDs `2562435`, `2562524`,
`2562632` all exited 0, joined and were proved gone before the next check.
Receipts/cutoffs/logs: `/tmp/kandev-child57-design-docs-20261006-receipt.json`
and `/tmp/kandev-child57-design-docs-20261006-<check>.log`. Coverage spawned no
child PID. Zero live handles at that handoff. ROOT subsequently authorized
recovery with existing Node 24.21.0 and the exclusive implementation window;
the original failure was preserved without replay.

Implementation: corrected evaluator passed (docs-only exempt/ok; projected hook
covered), missed inventory passed and one conditional frozen pnpm install passed.
Independent original RED: 17 valid causal failures and four passing controls;
one nested-StrictMode fixture error was corrected and affected RED then failed
on obsolete publication. A subsequent rendered commit-boundary RED proved an A
link under B's URL before passive loading. Final full causal GREEN: 22 passed in
two files. Named compatibility: 27 passed in four files once; their mocked
presentation/CRUD contracts were unaffected by the later row gate.

Final changed eslint, project typecheck, i18n:check and i18n:ratchet passed.
Routine test-group size repair and owned-file formatting reran affected checks.
No browser/build/E2E, broad local suites, backend/API, mutation-order or flag
change. Paired broader specs remain draft; this order implements AC004.8 only.
Normal active hook and actual changed-path coverage receipts are required and
retained in the live task plan before ready publication. MERGE NONE until
fresh hosted gates pass and ROOT gives its separate serial grant.

Public documentation: internal specs/plans only. No command, public API,
terminology, feature default or navigation change; public docs need no update.

## Risks

- Rejecting obsolete finally without settling abandoned loading can leave a
  cached return loading forever; transition/cleanup owns that settlement.
- An owner change must reset loaded identity even when the workspace ID matches.
- Mocked store selectors cannot establish this race; causal tests must observe
  the real store and links. Independent concurrent multi-consumer ownership and
  read-versus-mutation reconciliation remain outside this bounded correction.
