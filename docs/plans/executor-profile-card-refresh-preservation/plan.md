---
created: 2026-10-10
status: implemented
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
system_design:
  - ../../specs/executors/system-design/profile-editor.md
legacy_specs: []
---

# Implementation Plan: Preserve live executors during profile-card refresh

## Overview

One sequential work order repairs `ExecutorProfilesCard.refreshProfiles`
publication. The executor [profile-editor requirement](../../specs/executors/requirements/profile-editor.md)
and [design](../../specs/executors/system-design/profile-editor.md#profile-card-refresh-publication)
own AC-EXECUTORS-PROFILE-EDITOR-001.25 through .27. This extends the existing
vertical owner; task destinations and phone surfaces consume the same inventory.

ROOT reviewed the completed design package and later explicitly released
implementation to this same primary with the exclusive local-heavy lease.
The one work order is done; exact-head hosted delivery remains external.

## Evidence and root cause

Source at `1f73bee3eb69623396e821279633eaa2ef54d7fb` confirms that the card
captures `executors.items` in its React callback, awaits a profile GET after
DELETE or creation acknowledgement, then replaces the entire catalogue with
that captured array. Live executor/profile handlers write current state while
GET is pending. Republishing the closure can revert a sibling's saved name,
erase a newly created executor or restore a deleted one.

ROOT supplied qualified rendered-card/store/registered-WS/DELETE/GET evidence:
native handle 17662, actual join `7f1a1c`, exit 1, two causal RED cases and two
passing ordinary/failure controls, no stderr/setup/unhandled errors. The earlier
77970 attempt collected no tests and is excluded. This evidence is reported by
ROOT, not independently rerun here. Its protected proof source was not read,
copied or accessed; independent regression authoring follows implementation
release. No discovery scratch was created in this checkout.

## Scope

### In scope

- Card-local GET publication over current shared inventory, with target-profile
  observation and a component-local overlapping-read guard.
- Preserve unrelated metadata, profiles, membership and order; keep removed
  executors absent and preserve target non-profile fields.
- Independent real component/transport/store/WS/displayed-inventory regressions
  and existing card navigation controls.
- The owning requirement/design edits, this manifest and exactly one work order.

### Out of scope

- Global executor API, store or WS policy, snapshot revisions, journals,
  server-write arbitration, target per-field merge, retries and duplicate repair.
- Other save/delete/refresh writers, task selection or launch behavior,
  provider rules, plugin interfaces, runtime/desktop processes and dependencies.
- Markup, localization, layout, touch, scrolling, routes or breakpoint changes.
- Foreign work, including child103/PR4388, shared deps/caches/processes/refs and
  ROOT's protected discovery source. Preserve all such resources.

## Technical approach

Keep the existing DELETE/POST followed by `listExecutorProfiles` GET with
`cache: "no-store"`. Use `useAppStoreApi` for the captured provider's store.
An eligible result maps only its target over a current store read immediately
before existing `setExecutors`; no await separates read and write.

At GET start, advance a card-local sequence. Subscribe to only the target's
observed state through this store; retain sticky absence and profile-transition
flags. Compare ordered profile IDs/references, treating absent/empty lists as
equivalent. Ignore array/executor identity changes with identical entries,
because unrelated profile deletions clone those containers. Target metadata-only
updates must also allow refresh. Any observed target profile transition or owner
absence suppresses that result, even if the state later returns to its original
value. Keep the entire current target list on suppression. Latest-started read
wins within the card even if the newer GET fails; older reads cannot resume
publication. Other card publications are observed target transitions.

Dispose the invocation observer in `finally`. Retain swallowed refresh errors,
failed-delete behavior, real built-in/plugin creation dialogs and post-refresh
creation navigation, including rejected/skipped GET. The invocation remains
bound to its captured store. No new unmount cancellation or navigation lifetime
policy is introduced. This local conservative fence needs no new ADR.

| Consumer / boundary | Identity and transport | Preserved behavior | Evidence |
| --- | --- | --- | --- |
| Settings executor listing | Card target ID, profile DELETE/POST then GET | Current rows, owner metadata and provider gate | Real subscribed card and adapters |
| Legacy executor connection page | Same card and target ID | Existing opening/delete/create interactions | Shared card plus existing navigation tests; read-only caller audit |
| Built-in and plugin creation | Existing dialog callback and canonical route | Navigation after refresh; plugin eligibility unchanged | Real built-in dialog POST/GET/router; shared callback source audit |
| Task creation and subtasks | Shared inventory with owner name/type fallback | Current eligible destinations; removed choices absent | Real options hook and displayed labels with production projection |
| Other executor writers | Independent state owners | Existing contracts | No production changes or broad replay |

## Desktop and phone applicability

This correction changes state/data publication only inside an existing card.
Desktop and phone consume the same catalogue. Control order, markup, responsive
composition, touch targets, scroll ownership, copy and navigation are unchanged.
The mobile-parity state/data exception applies: rendered card plus transport,
store, registered-event and displayed-options integration supplies the user-flow
evidence. No new UI preview, Playwright suite or whole-app build is required.
Reassess this exception before making any interaction or geometry change.

## Tests

The [work order regression matrix](task-01-preserve-card-refresh.md#regression-matrix)
maps all three acceptance criteria. Independently author two causal RED cases
for sibling update loss and executor creation loss before correction, with
ordinary/failure controls passing. Extend to deletion/mixed inventory, target
metadata and absence, profile transitions/restoration, unrelated WS clones,
overlapping reads and failure/navigation controls. Keep actual card, provider,
store, registered handlers, adapters, router and options logic real; hold only
external fetch. Assert visible live choices before releasing GET and retained
choices afterwards, not only store contents or helper output.

## E2E tests

The real rendered component/transport/WS/store/options path is the bounded
user-flow verification under mobile-parity's explicit state/data exception.
No new browser runtime, mobile E2E, container run or frontend build is planned.

## Work orders

- [x] [Task 01: Preserve card refresh publication](task-01-preserve-card-refresh.md)

## Changed-path coverage

At the design checkpoint, inventory actual tracked modifications and untracked
paths from Git, without filtering to expected files. Validate the four artifacts
with the repository PR coverage helper and separately validate their one-order
traceability: a docs-only diff is `exempt`, not implementation proof. The actual
inventory and results are recorded below. After implementation, repeat over
actual paths and require `covered`, no errors, and exactly one work order.
Unexpected foreign paths are reported and preserved rather than hidden.

## Verification results

Documentation checkpoint passed at the discovery head. Actual tracked and
untracked inventory contains exactly these four paths:

- `docs/specs/executors/requirements/profile-editor.md`
- `docs/specs/executors/system-design/profile-editor.md`
- `docs/plans/executor-profile-card-refresh-preservation/plan.md`
- `docs/plans/executor-profile-card-refresh-preservation/task-01-preserve-card-refresh.md`

| Check | Actual exit / join | Result |
| --- | --- | --- |
| `python3 scripts/list-docs.py validate` | 0 / `e7f569` | 368 decisions and 1496 specifications validated |
| `python3 scripts/lint-spec-files.py --all` | 0 / `2fe01e` | All specification files passed |
| Initial whitespace/status and coverage launch | 127 / `d5c34f` | Whitespace clean, four paths present; bare node missing from PATH |
| Same actual-path coverage and one-order cross-reference preflight with existing Node 24.21.0 | 0 / `03e858` | Four paths, exactly one order, zero reference/errors; docs-only coverage exempt |
| Final catalog/spec/whitespace/status/actual-path preflight after documentation edits | Handle `52470`, exit 0 / `17cde8` | Original process joined; all gates pass, same four paths and one order |

The coverage helper's docs-only exemption is not product-test evidence.
The independent preflight resolved order-to-plan/design references, design and
plan requirement declarations, and all three criterion definitions. No foreign
paths were filtered out. The owning design is 32,595 bytes, below 32 KiB; no
size exception was added. Node was already installed through mise; no install
or dependency mutation occurred. Activate the existing Node 24 toolchain before
running the future package commands. Receipts remain outside Git at
`/tmp/kandev-child104-card-refresh-design-20261010/coverage.json`.

The design checkpoint above is historical. After the later explicit release,
the independent four-case RED selection produced two causal failures and two
passing controls. The final scoped run passed 24 tests across two suites, scoped
ESLint passed with zero warnings, and typecheck plus both i18n checks passed.
The [one completed work order](task-01-preserve-card-refresh.md#results) records
exact commands, original handles/joins and fixture corrections. Production
changed only the card's local refresh logic.

Implementation coverage uses the actual seven paths: the same four artifacts,
`apps/web/components/settings/executor-profiles-card.tsx`,
`apps/web/components/settings/executor-profiles-card-refresh.test.tsx`, and
`apps/web/components/settings/executor-profiles-card-refresh.test-helpers.tsx`.
The actual tracked/untracked preflight reports covered, exactly one work order,
and zero reference/errors (handle 9698, exit 0, join 293ae8). Catalog/spec and
whitespace checks also passed. No foreign path was filtered out; no broad suite,
Playwright or whole-app build was run. All local command handles were joined.
Normal hooks/publication and hosted validation follow these local results.

## Delivery constraints

Task `4c3d766f-5594-467a-94ec-026ff019c94a`, primary session
`d2b7f408-7274-4468-b293-e29d8208bf5f`. Preserve this primary, active model and
executor; no native delegates or new sessions. ROOT supervises the external
task plan directly; a rejected callback is not a progress gate. The documentation
lease was returned at the design handoff; implementation followed a later
explicit interrupt and its separate exclusive lease.

After release, use this order's scoped checks and normal hooks/publication.
Operational leases, handles and hosted delivery gates remain in the external
task plan and ROOT's instructions, without separate tracked procedural records.

## Risks

- Array identity alone would suppress valid refresh after unrelated WS clones;
  profile-entry comparison must be proved with real registered handlers.
- A final snapshot alone misses disappearance/change-and-restore. Sticky
  observation and guaranteed disposal are required on every transport outcome.
- Suppressing a contested whole target list can defer response-only changes.
  This deliberately avoids partial stale merging and imposes no server ordering.
- The existing owning design is close to 32 KiB; its condensed historical
  guidance must preserve existing contracts without expanding this work order.
