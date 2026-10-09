---
created: 2026-10-10
status: implemented
requirements:
  - REQ-EXECUTORS-OWNER-DELETION-001
system_design:
  - ../../specs/executors/system-design/executor-deletion.md
legacy_specs: []
---

# Preserve the current catalogue during executor deletion

## Overview

Correct accepted owner deletion on the active legacy executor settings route so
it removes only the target from current catalogue state. One sequential work
order independently reproduces the stale-array defect, implements the local
owning-store read, and verifies retained catalogue choices and existing controls.
ROOT completed actual-file review at DESIGN_READY and later explicitly
released this same primary with the exclusive heavy lease. The local repair and
its one work order are implemented; publication and hosted checks continue under
ROOT supervision without a merge grant.

## Ownership and assumptions

Executors own catalogue membership and owner lifecycle. The existing
[profile-editor requirement](../../specs/executors/requirements/profile-editor.md)
excludes owner deletion outside profile-card refresh; its save/refresh contracts
remain intact. This pair defines the separate owner-delete lifecycle rather than
expanding or polishing the nearly full adjacent design. The system boundary
does not change, so its README needs no edit. No new ADR is needed for reusing
the established current owning-store pattern.

Confirmed scope: accepted target-only owner removal, every current survivor's
fields/profile membership and current selector choices; exact confirmation,
system eligibility, transports, navigation, error propagation and dialog
behavior remain intact. Source confirms the active route, stale capture, real
HTTP/WS adapters, registered events, and selector projection. ROOT supplied
causal evidence; it is qualification, not this work order's independent TDD.
No material unresolved design choice remains.

## Qualified root cause

At `2d8711442bee996f70a0d5bb5f5997c64bbde1af`, `DeleteExecutorSection` captures
`executors.items`, awaits accepted deletion, and replaces the catalogue with
that captured array filtered by target ID. Registered live owner/profile creation
and unrelated owner removal while HTTP DELETE is held are undone at acceptance.

ROOT's receipt `/tmp/kandev-root-executor-delete-discovery-20261010/qualified-proof.json`
records one causal RED and two passing controls (ordinary accepted deletion,
mismatched exact lowercase confirmation): original handle 69764, exit 1, join
`6b2e7b`. Through the actual legacy route and SPA return, the actual selector
retained `retained-profile`, resurrected `removed-profile`, and lost
`created-choice`. The initial label-case fixture error is not causal evidence.
The protected candidate source must never be read, copied, moved or deleted.

## Scope

### In scope

- Successful `DeleteExecutorSection` publication in
  `apps/web/app/settings/executor/[id]/page.tsx`.
- Independent active-route/action-or-WS/store/registered-event/actual-selector
  regression and ordinary/exact-confirmation/system controls.
- One safe explicitly awaited rejected-HTTP-action control, with evidence limits.
- Four owning artifacts and accurate local verification/delivery results.

### Out of scope

- Backend, API/WS payloads, event writers, global store/provider registry,
  revisions, dependencies, framework or cleanup changes.
- Profile create/edit/delete/refresh and policy/connection saves, adjacent
  selection/default/launch policy, same-ID server arbitration, and request
  lifetime redesign.
- New catch/toast/retry or rejected-click error handling, layout/copy/touch
  changes, new browser suites, broad suites, builds, or unsolicited docs polish.
- Delegates, recursive tasks, new sessions/tabs/model switches and foreign or
  protected resources.

## Technical approach

Acquire `useAppStoreApi` inside `DeleteExecutorSection`. After successful
transport, read its current `executors.items`, filter only the submitted owner,
and synchronously call existing `setExecutors`, without an intervening await.
Remove only this section's obsolete captured-catalogue subscription. Preserve
the rest of the page and both transport branches.

| Boundary | Intended result | Evidence |
| --- | --- | --- |
| Active non-k8s legacy executor route | Real delete section and existing SPA return | Actual route/page/dialog/client router |
| No connected WS client | Same HTTP DELETE endpoint, no body, accepted 204 | Held external fetch and real action adapter |
| Connected WS client | Same `executor.delete` and exact `{ id }` | Held external request boundary |
| Registered executor/profile events | Live membership and survivor fields retained | Real handlers on the provider's real store |
| Desktop/phone task/subtask options | Current eligible surviving choices | Production owner fallback, real options hook and actual selector |
| HTTP rejection | Actual adapter rejects; no new handler behavior | Explicitly awaited adapter control; rendered rejection not claimed |
| System/k8s/profile routes | Existing exclusions and routing | System control and unchanged scoped route tests/source |

## Desktop and phone applicability

The existing dialog, route, selector combobox/touch drawer and settings scroll
composition are unchanged. This state/data publication correction qualifies
for mobile-parity's explicit component-test exception. Actual rendered route
and selector integration supplies bounded user-flow evidence; no new UI sketch,
phone Playwright test or whole-app build is required. Reassess if the scope
changes any layout, touch, scroll, navigation structure or viewport behavior.

## Tests

The [single work order](task-01-preserve-owner-delete.md#regression-matrix) maps
AC .1-.3 to the independently authored
`app/settings/executor/[id]/executor-delete-catalogue-publication.test.tsx`,
with a same-directory test helper if needed. Keep active route/page/form,
actual action API, provider store, registered handlers, real router and actual
selector intact. Do not derive fixtures from ROOT's protected source.

The initial RED selection contains a mixed-live accepted HTTP regression plus
ordinary-success and exact-guard controls. GREEN extends the same success
contract through WS and verifies survivor fields/profiles, earlier target
notification, system eligibility and explicitly awaited genuine HTTP rejection.
A failed rendered click is not safely awaited by React's event contract; no
rendered rejection PASS or new error handling is required or implied.

## E2E tests

The real rendered component/HTTP-or-WS/store/registered-event/router/selector
path is the bounded end-to-end user-flow check under the state/data exception.
It must assert committed SPA location and displayed eligible inventory after
acceptance, rather than only helper output or a mock router call. No new
Playwright runtime or browser build is planned.

## Work orders

- [x] [Task 01: Preserve accepted owner-delete publication](task-01-preserve-owner-delete.md)

## Changed-path coverage

Inventory actual tracked, staged and untracked changes without filtering to
expected files. Run `.github/scripts/pr-docs.cjs` `validateCoverage` against that
inventory and validate this package's one-order frontmatter references and
every AC definition separately. A docs-only `exempt` result is not product or
implementation evidence. After implementation require actual coverage `covered`,
zero errors and exactly one changed work order. Preserve and report foreign
paths rather than silently excluding them.

## Verification results

Lightweight design checkpoint at `2d8711442bee996f70a0d5bb5f5997c64bbde1af`:

| Check | Actual exit / join | Result |
| --- | --- | --- |
| `python3 scripts/list-docs.py validate` | 0 / `a16a17` | 368 decisions and 1506 specifications validated |
| `python3 scripts/lint-spec-files.py --all` | 0 / `66fe9e` | All specification files passed |
| `git diff --check`, status and size inspection | 0 / `7723bb` | No tracked/staged changes; only the four new artifacts; both specs below limits |
| Actual-path coverage, references, links and untracked whitespace using existing Node 24 | 0 / `4094f2` | Four actual paths, docs-only exempt, exactly one order, zero reference/coverage errors |

Actual inventory:

- `docs/specs/executors/requirements/executor-deletion.md`
- `docs/specs/executors/system-design/executor-deletion.md`
- `docs/plans/executor-delete-catalogue-preservation/plan.md`
- `docs/plans/executor-delete-catalogue-preservation/task-01-preserve-owner-delete.md`

The actual-path preflight read tracked, staged and untracked Git paths without
filtering; resolved order-to-plan/design and plan-to-design references; checked
design/plan requirement declarations, all three AC definitions, draft/pending
statuses, links, final newlines and whitespace. Receipt:
`/tmp/kandev-child108-executor-delete-design-20261010.RX4pOB/coverage.json`.
The docs-only exemption is not implementation evidence. No dependency install,
product test, package lint, build, hook or production/permanent test edit ran.
All lightweight design commands completed directly with no outstanding handles
or heavy lease at that historical checkpoint. The later explicitly released
[completed work order](task-01-preserve-owner-delete.md#results) records independent
RED, the minimal correction, 118 affected-suite passes and final 10-test/lint/
typecheck/i18n evidence with actual original handles and joins. All local child
process groups are absent at their recorded joins.

Implementation changes are exactly the four artifacts above plus:

- `apps/web/app/settings/executor/[id]/page.tsx`
- `apps/web/app/settings/executor/[id]/executor-delete-catalogue-publication.test.tsx`
- `apps/web/app/settings/executor/[id]/executor-delete-catalogue-publication.test-helpers.tsx`

Final prepublication documentation check: original exit 0 / join `785513`,
catalogue and all specification files passed, whitespace clean. Actual-path
coverage/one-order/link/whitespace preflight: original exit 0 / join `629af3`,
exactly seven paths, `covered`, zero errors, one completed work order and all
three criteria traced. No foreign path was filtered. Receipt:
`/tmp/kandev-child108-executor-delete-design-20261010.RX4pOB/implementation-coverage.json`.
The same check observed all eleven original local child groups joined and
currently absent; `local-process-absence.json` records the actual groups.
The first normal commit hook caught a new staged test-helper placeholder omitted
by the earlier unstaged ratchet. The helper now uses an existing translation key;
final native 25491 joined exit 0 / `2b5c50`: 10 new tests, scoped lint, typecheck,
i18n check and the staged ratchet all passed. Normal hooks remain enabled for
the next commit attempt. Authorized publication follows this checkpoint.

## Public documentation

Internal docs updated only. Public executor/profile creation and selection
instructions, lifecycle descriptions, API/WS contracts and screenshots remain
accurate. Catalogue continuity restores the existing choice outcome without
new user steps, terminology or operator configuration; no public page change is
needed. Search/reassess after implementation, without adjacent docs polish.

## Delivery constraints

Operational task/session identity, ROOT supervision, protected resources,
resource leases, original process handles and hosted validation checkpoint are
recorded in the external task plan. This manifest is implementation scope,
not a substitute for that live record. Do not implement from DESIGN_READY alone.
After ROOT's later explicit release, use the listed work-order checks, normal
hooks and authorized commit/ordinary push/PR. Return heavy resources and proceed
to hosted checks in that same turn under the supplied single-observer and
expected-head merge rules. No merge before ROOT's explicit squash grant.

## Risks

- Mocking the legacy page, action, registered writers, router or selector would
  bypass the qualified causal path and invalidate the regression.
- Owner-created payloads alone do not attach profiles. Tests must send the real
  follow-up profile event, and use valid non-system eligible fixtures.
- Rejected clicks can produce unhandled errors; the safe transport control must
  not be described as a rendered rejection test or authorize catching errors.
- A store captured from another provider or a read before awaiting would retain
  the stale-state defect. Read the section's owning current store at publication.
