---
status: draft
system: tasks
requirements:
  - REQ-TASKS-IMPORT-PROFILES-001
---

# Workflow import profile selection design

## Purpose and boundaries

The manual import flow resolves explicit step profiles before workflow persistence.
It preserves the direct-profile invariant for earlier-step session targets.
The existing matcher remains authoritative for automatic exact matches.
Selections use profile IDs, so duplicate descriptors cannot redirect a user's choice to another local profile.

No database migration or portable YAML version change is required.
Profile selection does not change unattended sync or legacy raw-YAML profile matching.
The later [portable profile matching design](workflow-portable-profile-matching.md)
extends the descriptor and synchronization contract, including version 3;
the statements here describe the original selection delivery.
One-time import name selection is shared by preview, browser submission, and raw-YAML/MCP import.
This is a local import extension. Its requirement and design preserve the rationale without a separate ADR.

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| 001.1, 001.2, 001.3, 001.7 | Preview and selection contracts |
| 001.4, 001.5 | Validation and persistence |
| 001.6, 001.9 | Client flow and recovery |
| 001.8 | Responsive composition |
| 001.7, 001.10, 001.11 | Shared workflow-name selection |

All identifiers in this table have prefix `AC-TASKS-IMPORT-PROFILES-`.

## Existing components

- `apps/backend/internal/workflow/service/service.go`: `ImportWorkflows`, `importSingleWorkflow`, `stepFromPortableWithMatcher`, and `validateWorkflowSessionTargets`.
- `apps/backend/internal/backendapp/services.go`: `buildAgentProfileMatcher` and `selectAgentProfileCandidate`.
- `apps/backend/internal/workflow/handlers/handlers.go`: `httpImportWorkflows` and workspace routes.
- `apps/backend/internal/workflow/controller/controller.go`: `ImportWorkflowsRequest` and controller delegation.
- `apps/web/app/settings/workspace/workspace-workflows-client.tsx`: `useWorkflowImportExport` currently sends YAML directly.
- `apps/web/app/settings/workspace/workspace-workflows-dialogs.tsx`: `ImportWorkflowsDialog` currently offers upload and paste.
- `apps/web/app/actions/workspaces.ts` and `apps/web/lib/types/http.ts`: HTTP actions and response types.

New import resolution code belongs in focused service, handler, hook, and component files beside these boundaries.
The backend wiring supplies a context-aware profile catalog to the workflow service.
The catalog lists eligible profiles and retrieves selected profiles using existing agent settings repositories and access rules.
Catalog failures propagate as retryable errors instead of appearing as an empty candidate list.

## Preview and selection contracts

Add `POST /api/v1/workspaces/:id/workflows/import/preview` with the existing raw-YAML request format and size limit.
This endpoint performs no persistence. It authorizes the destination before exposing any profile or workflow information.
It parses, normalizes, and validates the portable export with the existing model validation.
It applies current workflow-name deduplication before resolving direct step profiles.

The JSON response contains:

- `skipped`: workflow names excluded by existing deduplication.
- `profiles`: eligible candidates with `id`, profile label, agent display name, model, mode, and `updated_at`.
- `steps`: explicit direct-profile steps in workflows that will be imported.
  Each entry contains `workflow_index`, workflow name, `step_position`, step name, the requested descriptor, and optional matched profile identity.
  A matched identity contains `id` and `updated_at`.

Workflow index refers to the original document array. Step position refers to the validated portable position.
Names are display labels, not binding keys. Repeated descriptors do not share selection state.
Missing and empty optional descriptor fields follow existing matcher semantics.
Automatic matching preserves the enabled/global filter and oldest-candidate tie-break rule.
The catalog does not redefine runtime health, model fallback, or discovery policy.

Extend the existing import route with an explicit `application/json` request form:

```json
{
  "yaml": "version: 2\n...",
  "step_profile_bindings": [
    {
      "workflow_index": 0,
      "step_position": 2,
      "requested_profile": {
        "agent_name": "Codex",
        "model": "gpt-5.6-luna",
        "mode": "agent-full-access"
      },
      "profile_id": "selected-local-profile-id",
      "profile_updated_at": "2026-09-17T13:00:00Z"
    }
  ]
}
```

The browser submits bindings for every explicit direct-profile step, including automatic matches.
The backend distinguishes the JSON envelope by content type, not body guessing.
Existing raw YAML, including JSON-shaped portable exports submitted with the existing YAML content type, retains its current parsing path.
Both request forms retain the existing success shape: `created` and `skipped`.
Body limits apply to both forms. Oversized or truncated bodies return an explicit request error.

## Shared workflow-name selection

`ImportWorkflows`, `PreviewImportWorkflows`, and `ImportWorkflowsWithBindings` apply one document-order selection policy after authorization and whole-envelope normalization/validation.
Read destination workflows with `ListWorkflows(ctx, workspaceID, true)` so hidden names participate.
Use a request-local selection helper beside the import service code. It copies the existing-name set, walks the original workflow array, and returns retained original indexes plus skipped names.
An exact name enters the local seen set when its first eligible occurrence is retained.
Do not trim, case-fold, compact, reorder, or mutate the portable workflow array.
Selection is not a persistence operation or a concurrent-request reservation.

The raw path imports retained indexes through `importSingleWorkflow` and appends a name to `Created` only after successful creation.
It keeps its existing error and partial-storage-failure behavior; a failed first occurrence never selects a later duplicate as a fallback.
The preview passes retained indexes to `collectImportProfileSteps` and reports skipped names from the same selection.
The bound submission computes selection once from its freshly loaded destination state and passes those indexes to both `resolveImportProfileBindings` and `prepareImportedWorkflows`.
These stages must not independently consume or mutate a shared seen-name set: a second pass would otherwise skip the first pass's retained items.
Profile IDs and bindings remain keyed by original workflow index and validated step position.

`expectedImportProfileSteps` and `validateImportProfileBindings` continue to check the original whole document.
Unknown or duplicate binding keys and descriptor mismatches remain errors even for skipped occurrences.
After that structural check, only retained occurrences resolve profiles and prepare/persist steps.
Thus an omitted binding or an unavailable profile on a later duplicate cannot block the retained first occurrence.
Whole-envelope validation still rejects malformed steps or references in skipped entries before writes.
Retained entries keep existing profile, step-reference, and session-target validation.

Submission re-reads destination names rather than trusting preview as a reservation.
For unchanged destination state, preview and submission select identical occurrences; if a name was created after preview, submission skips it under the same policy.
Neither path updates or merges a skipped workflow or writes its steps.
No uniqueness index, migration, cross-request lock, or new batch transaction is introduced.
`importSingleWorkflow` and `sync_apply.go` keep their conversion and sync contracts.

## Validation and persistence

The new service entry point authorizes, parses, and validates the entire submission again.
It reloads existing workflow names and selected profiles before preparing any workflow.
It rejects duplicate binding keys, unknown positions, descriptor mismatches, and bindings on steps without explicit profiles.
Bindings for workflows now skipped by name are ignored after structural validation.
Every remaining direct-profile step requires exactly one binding.

Selected IDs must remain eligible and retain their submitted `updated_at` value.
The timestamp is a consistency guard, not an authorization credential.
Unavailable profiles use a generic unavailable reason without exposing private profile data.
Changed timestamps require renewed review even when a descriptor still matches.
The final service uses the loaded profile IDs directly. It never rematches the replacement's descriptor.

Prepare all workflow steps in memory, apply exact selected IDs, then run existing step, task-reference, and session-target validation.
Only after every candidate workflow passes validation does persistence start.
The prepared steps are the steps that persistence consumes. No second matcher pass can change them.
Extract preparation/persistence helpers only where they preserve existing import and sync callers.
`sync_apply.go` calls `importSingleWorkflow`, so new interactive requirements must not leak into that shared path.

Profile resolution failures return HTTP 409 with `code: workflow_import_profiles_required` and affected step keys and reason codes.
Reasons distinguish missing selection, unavailable profile, and changed profile. Malformed bindings return HTTP 400.
The legacy import path retains its error behavior.
The client uses structured fields, never an English error-string match.
The existing `ApiError` handling must retain structured details for this action, or the action must decode its response locally.

The no-write guarantee covers preflight, selection, and validation failures across the submitted batch.
Storage failures trigger compensating deletion of steps and the newly created workflow for the current item. This change does not introduce a cross-repository transaction.
After an uncertain network result, retry rechecks name deduplication before creation.

## Client flow and recovery

Extract `useWorkflowImportExport` state for import into a focused hook with states for editing, preview, resolution, and submission.
The Import action first requests preview. With no missing step profiles, it submits the returned exact bindings automatically.
Otherwise it shows all missing steps grouped by workflow, with an independent selector for each step.
The selected profile label includes agent, model, and mode so a different agent family is an informed choice.
Matched steps remain bound automatically. Initial-session and earlier-step targets do not get replacement pickers.

Editing YAML, uploading another file, changing destination, or starting a new import invalidates previous preview and bindings.
An import generation guard rejects stale preview, file-read, and submission callbacks.
Closing before submission performs no writes. Closing after submission does not imply server cancellation.
Disable dismissal and repeated submission during the final request, then restore normal dismissal when it settles.
Failures retain draft input. Profile conflicts refresh preview and preserve only selections whose IDs and revisions remain valid.
An empty candidate list shows profile-settings access in a new tab and a Retry action, preserving the import draft.
Loading uses a localized status region. Success uses the existing toast and workspace refresh.

## Responsive composition

The entry point remains workspace workflow settings, through upload or paste.
Desktop uses a bounded Dialog with grouped rows, searchable profile popovers, and fixed footer actions.
The YAML and profile dialogs share a desktop maximum width of 48 rem.
The file chooser uses a compact secondary button, with a 28 px desktop height and 44 px touch height.
The YAML editor has a useful initial height and bounded scrolling.
Profile popovers start closed and open only after an explicit user action.
Selected profile triggers use one truncated line with the full label available through their title.
The candidate list retains separate profile-name and descriptor lines in content-sized rows.
Phone uses an inset, nearly full-height Drawer because a batch can contain many profiles and long descriptors.
The same drawer switches between the missing-step list and a focused searchable profile list with Back navigation.
This avoids stacked drawers. A selection returns to the missing-step list without losing earlier selections.

The nearest shipped geometry exemplar is `components/kanban/mobile-menu-sheet.tsx` and its `ResponsiveMenuSurface`.
It contributes dynamic viewport sizing, a fixed header, and one internal scroll owner.
`workflow-session-target-surfaces.tsx` provides the searchable profile row and agent-logo precedent.
The import picker must not expose that component's session targets or lifecycle controls.

Use `useResponsiveBreakpoint`, `@kandev/ui` Dialog/Drawer/Command primitives, and shared state across viewports.
Keep the header and footer fixed. The active body owns `min-h-0`, vertical scrolling, and overscroll containment.
Use dynamic viewport height and safe-area padding. Long labels wrap without horizontal overflow.
Fine-pointer desktop controls use 28 px sizing. Phone and coarse-pointer controls have at least 44 px hit targets.
Focus enters the active surface and returns to its trigger after dismissal.
Back or Escape exits the profile subview before dismissing the import surface.

## Verification and documentation

Service tests cover unresolved and selected variants of the supplied Feature workflow, exact ID retention, and whole-batch validation before writes.
Handler tests cover both content types, preview authorization, limits, and structured conflict responses.
Hook tests cover stale results, independent choices, retries, empty candidates, and duplicate submissions.
Desktop and phone E2E tests import through the UI and verify persisted selections and PR's target after reload.
Phone tests also verify focused picker navigation, containment, scroll ownership, and touch targets.

Public documentation changes belong in `docs/public/workflow-import-export.md` with the implementation.
It must distinguish the browser selection flow from legacy raw-YAML and unattended imports.
All new UI text uses locale keys for English, Portuguese, and the three Chinese locales, plus generated pseudo-locale content.

The bounded [workflow import name-selection repair](../../../plans/workflow-import-name-deduplication/plan.md) covers shared selection only.
Its service regressions use real task/workflow repositories, real UUID-producing task-service creation, and SQLite foreign keys; HTTP and MCP boundary tests cover their separate request forms.
No frontend composition or control change is needed for this backend-owned selection correction.
The existing profile-selection delivery record remains historical; this amendment does not promote the broader draft requirement or design.

## Related contracts

- [Requirements](../requirements/workflow-import-profile-selection.md)
- [Session lifecycle and recipients](workflow-profile-session-lifecycle.md)
- [Profile disable behavior](../../agents/requirements/profile-disable.md)
- [Runtime model strictness](../../../decisions/2026-09-15-explicit-profile-model-strictness.md) remains separate from portable profile matching.
