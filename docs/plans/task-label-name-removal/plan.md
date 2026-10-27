---
created: 2026-10-26
status: implemented
requirements:
  - REQ-TASKS-LABELS-002
system_design:
  - ../../specs/tasks/system-design/labels.md
legacy_specs: []
---

# Implementation plan: Exact-name task label removal

## Overview

Make an accepted label name removable from one task by moving the production
client's removal identity into a query parameter. One sequential vertical work
order owns the registered route, client, regressions, shared desktop/phone
component behavior, and public documentation. Preserve the old endpoint for
existing clients.

## Scope

### In scope

- REQ-TASKS-LABELS-002 and AC-TASKS-LABELS-002.1 through .6.
- Add collection DELETE with one query-encoded exact name; retain legacy DELETE.
- Preserve workspace/task authorization, catalog, unrelated labels and tasks.
- Independently authored regression tests, shared picker behavior through the
  real optimistic hook, plus a small Office task-doc update.

### Out of scope

- Colors, catalog management, task JSON migration, filtering, CLI/MCP expansion.
- Global Gin behavior, release toggles, router wildcard changes, UI redesign.
- Adjacent error handling, cache/event redesign, unrelated file-link behavior.
- New browser tests/builds and legacy phone layout audits.

## Evidence and root cause

Source inspection is at `c5ff8b7bf7db0c392986d7124711a642a8bca592`.
Pre-planning reproduction reports POST `area/backend` and `a/b/c` each
returning 201 with one persisted association, followed by escaped-path DELETE
404 with the association retained. `urgent` and `needs review` controls remove
with 200 and no remaining association. That was an actual registered handler,
real LabelService, and real FK-enabled settings/task/Office SQLite schema run.
The implementation regressions will be independently authored against the
public registered route and persistence contracts.

Source independently confirms `LabelsPicker` permits slash names,
`removeLabel` encodes them into a single segment, `RegisterRoutes` expects
`:labelName`, and the existing service/repository perform exact-name/pair
resolution. Decoded slashes prevent route dispatch; persistence is not the
primary fault.

## Technical approach

Follow [the paired design](../../specs/tasks/system-design/labels.md), especially
Removal API and compatibility, Authorization, and Desktop and phone behavior.
Add `DELETE .../tasks/:taskId/labels?name=...` under the current Office group.
Use `url.ParseQuery` and require exactly one nonempty name. Change only the
production client's serialization to `URLSearchParams`; reuse the existing
service. Keep legacy route extraction/response and request options intact.

Compatibility cases:

| Caller/input | Expected behavior | Evidence |
| --- | --- | --- |
| Production client, `area/backend`, `a/b/c` | Query DELETE removes exact association | Route/SQLite, production API and shared picker tests |
| Query, literal `%2F`, `a+b`, `needs review`, stored edge spaces | One decode; distinct exact identity | Route/SQLite plus API unit |
| Existing path client, `urgent`, `needs review`, literal percent/plus | Existing 200 response and exact deletion | Registered legacy-route controls |
| Missing/empty/repeated/bad escape query | 400, no mutation | Registered route regression |
| Absent valid label or task association | 200, no catalog creation/deletion | Route/SQLite |
| Foreign or mixed workspace/task | Existing scope denial before dispatch | Office scope tests |

## Surface and mobile-parity boundary

No rendered composition changes; ASCII previews are unnecessary for this
transport-only package. The same Labels row and chip actions in Office Simple
detail remain on desktop and phone. Mobile-parity's narrow state/data exception
applies because only API serialization changes; no layout, touch, scrolling,
navigation, or breakpoint behavior changes. Preserve the current presentation.

Prove the desktop/phone outcome by combining actual production-client request
tests, registered-route/real-persistence deletion and reread tests, and real
`LabelsPicker`/optimistic-hook component success/failure tests entered through
click, touch-pointer and keyboard actions. This is a bounded decomposition of
AC .6, not a blanket UI waiver. It proves shared behavior and durable state;
it does not claim browser reload, rendered phone geometry, or managed E2E
coverage. No legacy phone layout audit/redesign is required.

## Tests

- `apps/backend/internal/office/labels/handler_test.go` (new):
  `TestTaskLabelRemovalNameIdentity`, `TestTaskLabelRemovalLegacyCompatibility`,
  `TestTaskLabelRemovalInvalidQuery`, and
  `TestTaskLabelRemovalIsolationAndIdempotency` cover AC .1-.4 with actual routes,
  real service and actual FK-enabled schema constructors.
- `apps/backend/internal/backendapp/office_scope_test.go`: extend the existing
  mixed-param case table to exercise the new route under owner/foreign and
  agent callers. Structural `office_scope_completeness_test.go` checks include
  the real registered route and mixed-workspace denial (AC .5).
- `apps/backend/internal/backendapp/office_label_scope_test.go` (new):
  `TestOfficeTaskLabelRemovalScope` proves the new query route permits owners
  and confines browser/agent callers across foreign, mixed and missing task
  cases (AC .5).
- `apps/web/lib/api/domains/office-label-removal-api.test.ts` (new): actual
  `removeLabel` request method, encoding, options and non-2xx propagation
  (AC .1, .3, .4, .6).
- `apps/web/components/task/simple/components/labels-picker.test.tsx` (new):
  remove exact selected name, preserve neighbor chips, mouse click,
  touch-pointer and Enter/Space entry, and actual optimistic rollback/toast on
  rejected transport through the real hook (AC .6).

## Existing E2E baseline

`apps/web/e2e/tests/office/labels.spec.ts` is an existing API-oriented baseline
for ordinary label operations. Preserve it and its legacy helper. It does not
cover slash removal or browser/reload outcomes. No new E2E files, managed E2E
runs, or browser builds are required under the state/data exception above.

## Work orders

- [x] [Task 01: Remove an exact label name compatibly](task-01-remove-exact-label-name.md)

## Verification results

Implemented on 2026-10-26. Exact targeted commands and results are in
[Task 01](task-01-remove-exact-label-name.md#results).

- Independent backend RED returned 404 for collection DELETE; legacy controls
  passed. Production API/picker RED had 11 expected request-path failures and
  two passing controls.
- Backend label regressions and scope/route-completeness/multi-ID checks passed
  with the race detector and FK-enabled real schemas.
- Production API tests: eight passed. Real picker/optimistic-hook tests: five
  passed, including click, touch-pointer, Enter/Space and failure rollback.
- Canonical typecheck passed at 6144 MiB after correcting owned test-only type
  errors. Changed-Go lint reported zero issues; frontend lint with zero allowed
  warnings passed after extracting a repeated test literal.
- Staged i18n ratchet passed. Catalog/specification validation, all 63 public-doc
  validator tests, and validation of 54 published pages passed. The earlier
  design validation also passed all 36 specification-linter tests.
- Offline PR-documentation coverage for the actual implementation paths is
  `covered`, with zero errors. Reference/link and diff whitespace checks passed.

No managed E2E, browser reload or phone geometry check was run. The explicit
state/data exception above defines the decomposed desktop/phone evidence.

## Risks

- Query double decoding can silently delete a different label; keep collision
  cases attached together and verify persisted IDs.
- A new route outside the Office scope group would bypass established guards;
  retain structural route coverage and behavioral multi-ID tests.
- Component evidence cannot certify rendered phone geometry or a browser
  reload; preserve the explicit state/data exception and evidence limits.
- The current service treats lookup errors as an absent label. Preserve that
  existing policy here; do not silently expand into service error remediation.
