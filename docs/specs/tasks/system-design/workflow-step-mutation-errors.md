---
status: current
system: tasks
created: 2026-11-13
requirements:
  - REQ-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001
---

# Workflow step mutation error design

## Purpose and boundaries

The workflow HTTP adapter translates domain errors into stable status codes and safe messages.
The correction uses error identity at that boundary.
Task authorization and executor admission continue to own their existing decisions.

This design covers `writeStepMutationError` in `apps/backend/internal/workflow/handlers/handlers.go` and its five HTTP callers.
The callers handle ordinary create, update, delete, reorder, and template application.
WebSocket and MCP adapters keep their existing behavior.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001` | Error identity, HTTP classification, Verification boundary |

## Error identity

`models.ErrWorkflowStepNotFound` identifies a missing step from `Repository.GetStep` and from ordinary repository update/delete writes that affect zero rows.
`service.ErrNotVisible` already identifies a concealed resource rejection.
Both identities survive wrapping through `errors.Is`.

Ordinary update/delete writes can lose a race with another request after the
handler or service has read the step. Wrap the step sentinel at those zero-row
write boundaries so the shared HTTP mapper preserves the genuine 404. Keep the
version-fenced update's `ErrTaskVersionConflict` result unchanged.

Add `models.ErrWorkflowTemplateNotFound` with the existing base text `workflow template not found`.
In `Repository.GetTemplate`, wrap that sentinel only for `sql.ErrNoRows`.
Keep its existing resource-ID context and preserve every other database or decode error.
No SQL, schema, or repository interface change is required.

The task repository already supplies `repoerrors.ErrWorkflowNotFound` and `repoerrors.ErrWorkspaceNotFound`.
The workflow mutation policy can propagate these identities.
Use them without converting arbitrary error text into a resource miss.
`repoerrors.ErrExecutorProfileNotFound` identifies a missing executor profile.
Map a direct profile miss to a safe resource-specific 404 without exposing its ID.
The existing executor-mutation-denial check takes precedence when it wraps this profile error.

Update the `ErrNotVisible` comment in `service/access.go` to describe its identity-based handler contract.
Leave `IsNotFound`, `isNotFoundError`, and access normalization behavior outside this correction.
Those helpers also serve authorization and read surfaces.

## HTTP classification

Preserve the existing policy checks before resource classification.
Preserve the existing validation classifier and its precedence.
Replace the broad `strings.Contains(msg, "not found")` branch with explicit `errors.Is` cases.
Do not replace it with `service.IsNotFound`, which also accepts arbitrary error text.

| Error identity or class | HTTP status | Response |
| --- | --- | --- |
| `service.ErrWorkflowReadOnly`, `service.ErrWorkflowWorkspaceReadOnly` | 409 | Existing read-only message |
| `service.ErrNotVisible` | 404 | `Step not found` |
| `taskmodels.ErrWorkspaceDefaultExecutorUnavailable` | 409 | Existing safe message and `error_code` |
| `taskmodels.ErrExecutorMutationDenied` | 403 | `executor mutation denied` |
| `models.ErrInvalidWorkflowStepOrder` or existing recognized validation | 400 | Existing validation message |
| `models.ErrWorkflowStepNotFound` | 404 | `Step not found` |
| `models.ErrWorkflowTemplateNotFound` | 404 | `Template not found` |
| `repoerrors.ErrWorkflowNotFound` | 404 | `Workflow not found` |
| `repoerrors.ErrWorkspaceNotFound` | 404 | `Workspace not found` |
| `repoerrors.ErrExecutorProfileNotFound` | 404 | `Executor profile not found` |
| Any remaining error | 500 | `Failed to save step` |

The table defines precedence from top to bottom.
An executor denial can wrap a missing-profile error without losing its 403 classification. A direct profile miss uses the resource-specific 404; neither response exposes a profile ID.
A concealed resource rejection can wrap another error without disclosing its resource type.
Unknown errors have no independent HTTP status to forward.
They use the existing server-error response and retain the cause in existing logs.

## Failure, security, and compatibility

Keep authorization before mutation and keep the existing success-event publication paths.
The classifier must not retry an operation, change stored rows, or publish an event.
Do not add a new general-purpose error framework or expose `err.Error()` in the default branch.

Missing-template HTTP application keeps 404 and gains the correct template message.
Update the HTTP assertion in `TestCreateStepsFromTemplate` accordingly.
Its WebSocket assertion remains unchanged because that adapter does not call this mapper.

The correction adds no rendered UI, translation keys, runtime flags, migrations, or metrics.
Existing handler logs retain the full cause through `zap.Error(err)`.

## Verification boundary

Use three focused forms of evidence:

1. `repository/template_errors_test.go` proves the new missing-template identity and preserves genuine storage failures.
2. `handlers/step_mutation_errors_test.go` proves the classifier table, wrapped identities, and misleading text without recognized identity.
3. The same handler test file drives registered Gin routes with the real controller, service, and SQLite repository.

For route evidence, create an existing step and obtain HTTP 200 for it first.
Then inject a dependency failure through `Service.SetExecutorMutationChecker`.
Exercise create, update, delete, and reorder with otherwise valid inputs.
Assert the response, unchanged stored steps, and no successful mutation events.
Template application uses `SetWorkflowMutationReadOnlyChecker` to inject its pre-write dependency failure.
For update/delete races, use the executor mutation checker to delete the step
after the handler's successful read and before the repository write. Assert a
sanitized 404 and no successful mutation event. Repository tests cover both
ordinary zero-row writes; version-fenced updates must keep returning
`ErrTaskVersionConflict`.
Also cover actual missing steps, missing templates, typed workflow/workspace policy misses, and direct executor-profile misses through HTTP.

This provides end-to-end evidence at the affected HTTP boundary without a browser fixture or a live developer instance.
Existing workflow scope tests preserve the foreign/missing response equivalence.

## Related sources

- [Requirement](../requirements/workflow-step-mutation-errors.md)
- [Implementation plan](../../../plans/workflow-step-mutation-errors/plan.md)
- [Workflow step ordering design](workflow-step-ordering.md)
- [Executor ownership design](../../executors/system-design/per-user-ownership.md)
- [Opt-in authentication decision](../../../decisions/2026-07-24-opt-in-authentication.md)
