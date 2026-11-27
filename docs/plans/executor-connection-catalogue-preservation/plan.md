---
created: 2026-10-10
status: implemented
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
system_design:
  - ../../specs/executors/system-design/profile-editor.md
legacy_specs: []
---

# Implementation Plan: Preserve executor choices during connection refresh

## Overview

One sequential work order repairs successful SSH and Remote Docker connection
save publication. The existing executor [requirement](../../specs/executors/requirements/profile-editor.md)
and [design](../../specs/executors/system-design/profile-editor.md#connection-save-catalogue-publication)
own AC-EXECUTORS-PROFILE-EDITOR-001.21 through .24. Task-start and phone surfaces
consume the same catalogue; no separate Tasks or UI requirement is created.

This package is a design checkpoint, not implementation authorization. A later
explicit ROOT release to this same primary and separate local-heavy grant are
required before permanent tests, production changes or execution of checks.

## Admission and evidence

CHILD102 task `678bbbe6-330d-4ba2-a83c-86e3ab30def9`, primary session
`b4f20f36-27d5-4b2b-81bb-85333d9c1e1a`. Keep this primary session and its model.
No delegates, recursive tasks, new sessions, model switches or persistent task
creation. ROOT reviews the four artifacts and sends a later implementation
request. Do not ask for another approval or advance automatically.

ROOT's read-only evidence is
`/tmp/kandev-root-ssh-save-refresh-discovery-20261010/qualified-proof.json`.
It qualifies actual main `64c34321d2d3843851116f87fec1a5b8b48cd32e`, native
50823, joined chunk `579c30`, exit 1: two causal RED cases (created destination
lost; deleted sibling resurrected), failed-refresh control PASS. It used the
real hook, `createAppStore` and `registerExecutorsHandlers`. This metadata was
read; the protected `candidate.test.tsx` was not accessed. Never read, copy,
import, edit, remove or replay that protected candidate. Independently author
regressions after release; do not replay ROOT's reproduction.

Confirmed by source: successful `listExecutors` calls `setExecutors` with its
entire stale array. Real executor and profile handlers publish to current state
while that request waits. `updateExecutor` returns void; the server PATCH DTO
does not attach profiles, whereas the list does. No backend change is needed.

## Scope

### In scope

- The shared connection-save hook and independently authored hook/caller tests.
- Retain all current memberships and profiles, sibling changes and target
  metadata; publish only eligible target name/config fields.
- Retain refreshed normalization, submitted fallback, save rejection and awaited
  `onSaved` behavior, including observations during PATCH and GET.
- Extend the existing owner pair and exactly this manifest and one work order.

### Out of scope

- Backend, APIs, global store/WS policy, DTOs, config submission redesign,
  server-write arbitration and unrelated refresh/save writers.
- Layout, localization, routes, phone interaction, task selection/launch logic,
  browser infrastructure, new dependencies and resource cleanup.
- PR4382/child101 and any foreign edits, processes, deps or caches. Never touch
  paused task `a6032d95` or protected anonymous volume
  `2c48e791f0a8b8e64e6ecd30db0ede17388b572d4a303d39e2e0ee3fa7573ea7`.

## Technical approach

`hooks/domains/settings/use-save-executor-connection.ts` retains PATCH followed
by list refresh. It scopes publication to the current target and reads current
items immediately before the synchronous write. Do not remove refresh or add
a generic stale-response framework. Preserve every profile and non-target
executor from that current read, including an empty catalogue.

Use one invocation-local store observer from before PATCH through publication.
Track semantic name/config-key transitions and target absence; key presence is
distinct from a value. Touched keys remain protected after a change-and-restore.
Keep current values for touched fields, apply refreshed target values to
untouched fields, and apply submitted values only when refresh fails or omits
the target. A profile-only update must still allow normalized fingerprint
publication. A deleted or replaced target must never receive this old result.
Dispose observation on all paths before awaiting `onSaved`; callback failure
must propagate without entering fallback or invoking the callback again.

Do not alter the two active callers. Add their rendered regressions in the owned
test file; a proven caller defect requires a ROOT scope checkpoint before any
caller production edit. Keep SSH route reload/remount and Remote Docker's
non-connection config builder semantics. Audit existing tests that mock the
provider/API; such mocks are not causal evidence for this repair.

| Caller | Transport / identity | Preserved behavior | Evidence / fallback |
| --- | --- | --- | --- |
| SSH page | REST PATCH then list GET, captured executor ID and AppStore | Trust gate, shared save, reload, normalized pinned fingerprint | Real page/card/coordinator; failed list uses guarded submitted fields |
| Remote Docker connection | Same hook, builder retains non-SSH config | Remote daemon test endpoint, authority notice, pinned remount | Real section/card/store subscriber; same fallback |
| Task creation and subtasks | Current profiles with executor-name/type fallback | Current names and existing provider eligibility | Actual options hook and rendered labels; deleted choices absent |
| Other executor writers | Do not call this hook | Existing behavior | Read-only compatibility; no migration |

## Observable desktop and phone states

The following previews describe data outcomes in existing surfaces, not new
markup or pixel geometry. UI-01 is the existing settings connection card and
UI-02 is the shared task-start destination content. Desktop retains its settings
shell/task dialog; phone retains direct settings navigation and its existing
task-start surface. Scroll owners, control order and primary actions are unchanged.

```text
UI-01: connection settings after successful save (desktop and phone)
[Connection]  Pinned fingerprint: SHA256:server-normalized
[Test connection]  [Trust this host]  [Save changes]

UI-02: task-start destinations after refresh settles
Before (qualified stale publication):  old sibling; new choice missing
After (desktop dialog / phone existing picker):
  Saved host / retained current profile
  Newly created host / new eligible profile
  Renamed current sibling / current profile
  Deleted host or deleted profile: absent
```

AC .21/.22 require these membership and normalized-value outcomes. AC .23/.24
retain existing successful and failed save behavior. The labels are illustrative;
production copy and layout do not change. No new phone drawer or desktop control
is planned. Apply mobile-parity's state/data exception, with real rendered caller
and options evidence. Reassess if implementation requires layout, touch,
scrolling, navigation or viewport-dependent behavior changes.

## Tests

The [work order](task-01-preserve-connection-catalogue.md#regression-matrix)
names independently authored cases for all four criteria. Hold external
transport only; keep the hook, API adapters, store, registered executor/profile
handlers, callers, coordinator and options logic real. First produce RED for
creation loss and deletion resurrection, with a passing failed-refresh control.
Then cover normalization, profile-only changes, target changes/clears/restoration,
PATCH-window events, missing/deleted targets, callback failure and separate stores.

## E2E tests

The real rendered caller plus transport/store/handler/options integration is
the package's user-flow evidence. No new Playwright, runtime/container runs or
browser builds are needed for this state-only repair under mobile-parity's
explicit exception. Existing layout/touch contracts remain untouched; browser
verification becomes a scope question only if that assumption changes.

## Work orders

- [x] [Task 01: Preserve connection-save catalogue publication](task-01-preserve-connection-catalogue.md)

## Verification results

Implemented after ROOT's explicit release and exclusive local-heavy grant.
The one work order is done and records exact commands, failures/corrections,
results and actual joins. Independent RED reproduced the two catalogue failures
with its failed-refresh control passing. The exact affected run passed 57 tests
across seven suites; the final changed suites passed 31 tests after fixture
corrections. Scoped ESLint, repository typecheck with required generation, both
i18n checks, Python catalog/specification checks, whitespace and actual
eight-path documentation coverage passed. Coverage reports `covered`, no errors,
and exactly one work order. One frozen-lockfile dependency install ran from
`apps/`; no backend, caller production, API or global store changes were needed.

Normal hooks/publication and hosted validation follow these local results.
Operational receipts and live handles are retained outside Git at
`/tmp/kandev-child102-connection-design-20261010/checkpoint.json`.

## Delivery gates

After later implementation release, run only the work-order checks under ROOT's
local-heavy grant, normal hooks and publication without bypass. Preserve actual
native handles and joins. Use one PR observer (GNU timeout 90m, kill-after 10s,
60s cadence); no duplicate polling. Required six contexts and actual Backend,
Frontend and E2E parents must succeed, with no actionable findings. Assess
substantive existing reviews and the later diff; do not request fresh full bot
reviews on each addition/push/SHA. Focused rereview requires concrete material
change. No optional style/docstring push, main-drift rebase or passing broad-suite
replay. ROOT's serial static-compatibility grant is required before one normal
expected-head squash merge, without admin or branch deletion. These gates are
recorded for recovery, not released by this design turn.

## Risks

- A snapshot comparison alone misses change-and-restore; observation must start
  before PATCH and survive until publication, then release on every outcome.
- A config-object or target-object identity check would incorrectly block
  normalization on profile-only updates; track actual fields and key presence.
- This preserves observed client changes, not unobserved server ordering. A
  stale Remote Docker full-config submission remains outside this repair.
- The void update adapter requires retaining GET and submitted fallback; neither
  fallback nor a callback error is evidence of server normalization.
- Existing shared owner design is near its size limit; keep the added section
  within 32 KiB and avoid unrelated restructuring.
