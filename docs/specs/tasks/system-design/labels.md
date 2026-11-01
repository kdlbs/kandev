---
status: current
system: tasks
requirements:
  - REQ-TASKS-LABELS-002
---

# Task labels system design

## Purpose and boundaries

This design covers exact-name task-label removal in the
[owning requirement](../requirements/labels.md). The tasks system owns the
association even though implementation lives under Office. The broader label
catalog remains described by the migrated requirement; this repair does not
implement unfinished catalog, color, filter, CLI, or persistence features.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-TASKS-LABELS-002.1, AC-TASKS-LABELS-002.3, AC-TASKS-LABELS-002.4 | Removal API and compatibility |
| AC-TASKS-LABELS-002.2 | Service and persistence |
| AC-TASKS-LABELS-002.5 | Authorization |
| AC-TASKS-LABELS-002.6 | Desktop and phone behavior |

All rows implement `REQ-TASKS-LABELS-002`.

## Existing boundary and defect

`labels.RegisterRoutes` registers a DELETE route ending in
`/tasks/:taskId/labels/:labelName`. The browser's `removeLabel` in
`apps/web/lib/api/domains/office-extended-api.ts` puts `encodeURIComponent(name)`
in that path segment. The Gin router uses the decoded path under current
configuration, so a name such as `area/backend` becomes multiple path segments
and does not reach `Handler.removeLabel`. `LabelsPicker.handleAdd` accepts any
trimmed nonempty name; creation does not impose a slash prohibition.

`LabelService.RemoveLabelFromTask` already resolves the label by workspace and
exact name and removes the task/label junction. Fix the transport boundary
without changing this identity or the global router configuration.

## Removal API and compatibility

Add this route on the existing authenticated Office group:

```text
DELETE /api/v1/office/workspaces/:wsId/tasks/:taskId/labels?name=<query-encoded-name>
```

Require exactly one `name` value with nonzero length. Parse `URL.RawQuery` with
`url.ParseQuery` and reject parsing errors, missing/empty names, or repeated
names with HTTP 400 and the existing JSON error envelope before calling the
service. Ignore unrelated well-formed query keys. No trimming, case folding,
second unescape, split on slash, or query-to-path conversion is allowed. A
space-only stored name remains addressable; this endpoint does not add creation
validation. Standard query decoding treats `+` as a space, so callers must
query-encode a literal plus as `%2B`.

Change the production browser `removeLabel` to serialize a single `name` using
`URLSearchParams`. Keep its function signature, request options, DELETE method,
and error propagation. For example, `area/backend` serializes as
`name=area%2Fbackend`; the distinct literal name `area%2Fbackend` serializes as
`name=area%252Fbackend`.

Retain `DELETE .../labels/:labelName` and its existing parameter extraction and
responses. Both route forms delegate to the same service operation; successful
removal returns HTTP 200 with `{"ok":true}`, including a valid absent name or
association. The old route continues serving existing simple/space-containing
callers. It is not extended to decode slash paths. Do not replace it with a
wildcard, toggle Gin raw-path behavior, or reinterpret a path name as a query.
The collection GET/POST and catalog DELETE by label ID remain separate routes.

This local additive endpoint is chosen over a DELETE body (avoids dependence
on body handling in clients/intermediaries) and over router-wide raw-path
changes (avoids altering unrelated route decoding). An ID-only replacement
would require changing existing name-based callers. These bounded tradeoffs
are fully preserved by this design and its work order; no global ADR is needed.

## Service and persistence

Keep `LabelService.RemoveLabelFromTask`, `GetLabelByName(workspaceID, name)`, and
`Repository.RemoveLabelFromTask(taskID, labelID)` as the association boundary.
The SQL deletes only a matching `office_task_labels` pair; it must not call
`DeleteLabel`, update `office_labels`, create a label, or modify task JSON.
No schema, data migration, event protocol, flag, or new cache is required.
Existing service error handling is outside this transport repair.

Regression evidence must inspect actual persisted associations and catalog IDs
with real settings, task, and Office SQLite schemas and foreign keys enabled.
Use two tasks sharing a label plus a same-name label in another workspace and
near-collision names on the selected task. A 200 status or optimistic chip
disappearance alone does not establish successful exact removal.

## Authorization

`backendapp.registerOfficeRoutesWithScope` installs
`officeWorkspaceScopeMiddleware`. It resolves both `:wsId` and `:taskId` and
rejects foreign ownership and mixed-workspace routes. The query name is data,
resolved within the authorized workspace, not another resource ID. Register the
new route on that same group; no middleware bypass, handler-specific ownership
policy, resolver exemption, or scope-table broadening is needed. Keep the
legacy labelName subresource declaration while its route remains registered.

Extend the existing mixed-parameter test cases and structural route tests to
cover the new route for owner success, foreign caller/resource, mixed
workspaces (including a caller owning both), and agent JWT confinement.

## Desktop and phone behavior

The affected entry is `/office/tasks/:id` Simple mode: `OfficeSimplePane` mounts
`TaskProperties`, whose Labels row renders `LabelsPicker`. Inline chip removal
and the open picker call the same `handleRemove(name)` and
`useOptimisticTaskMutation`. Keep its exact-name filter, optimistic update,
rollback/toast, and reload hydration behavior. No copy or markup changes are
needed.

Apply mobile-parity's narrow state/data-boundary exception: only API
serialization changes. Rendered layout, touch behavior, scrolling, navigation,
and responsive composition do not change. Keep the existing shared picker and
optimistic hook; do not add a phone variant or audit/redesign the legacy simple
pane. The curated `components/task/task-layout.tsx` is the advanced layout's
mobile exemplar, not a layout dependency of this shared transport boundary.

Decompose AC-TASKS-LABELS-002.6 into executable evidence at the boundaries that
implement the outcome: production-client tests prove exact serialization and
error propagation; registered-route/real-persistence tests prove durable
association removal and authoritative rereads; real `LabelsPicker` component
tests with the real optimistic hook prove success, rollback/toast, neighboring
chip preservation, mouse click, touch-pointer entry, and Enter/Space activation.
Exercise click and touch through the shared component handlers, rather than
mocking removal or the optimistic hook. Component tests do not establish phone
geometry or browser reload behavior; persistence rereads establish the durable
state that the unchanged reload path consumes. Record these limits explicitly.

This bounded combination satisfies the exception for the desktop/phone outcome
without new managed E2E, browser builds, layout checks, or reload E2E. Existing
`apps/web/e2e/tests/office/labels.spec.ts` is an API-oriented label baseline; it
does not cover slash removal or prove the picker/reload user flow. Preserve it
without claiming new slash/browser coverage.

## Validation and documentation

Use the test matrix and exact commands in the
[single work order](../../../plans/task-label-name-removal/task-01-remove-exact-label-name.md).
The principal regression is the registered DELETE route plus real service and
SQLite, not a service mock or manually injected Gin params. Preserve old-route
controls and nearby literal escape/plus/space names to detect double decoding.

Public documentation currently mentions Labels in
`docs/public/office-tasks.md` (how-to); no dedicated public label-removal API
reference was found in `docs/public`, the root README, or screenshot catalog.
During implementation add a short task-detail paragraph explaining removal of
names such as `area/backend`, retention on other tasks/catalog, and reload
persistence. Link an optional compact API-reference subsection describing the
query endpoint, query encoding, 400 validation, 200 idempotency, and retained
legacy path route. Leave the experimental Office availability/status intact.
No screenshot or navigation change is needed. Do not publish planned behavior
as shipped during this design-only turn.

## Implementation plans

- [Exact-name task label removal](../../../plans/task-label-name-removal/plan.md)
