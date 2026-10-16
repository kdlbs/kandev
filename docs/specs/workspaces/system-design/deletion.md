---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-DELETION-002
---

# Workspace deletion system design

## Purpose and boundaries

This design specifies the shared Settings deletion acknowledgement publication
owned by [workspace deletion](../requirements/deletion.md). Requirement 001's
complete-deletion contract remains unchanged. Its Office/native-home destination
does not describe the shared Settings editor, which already returns to
`/settings/workspaces`. The correction preserves that distinction.

The existing [settings update design](workspace-settings-updates.md) owns Save
and Add Workspace publication. Its earlier delete-draft inventory was read-only
and supplies no deletion guarantee. This design qualifies that independent
deletion callback without extending either previous implementation package.
Backend cascade, Office deletion, Settings bootstrap and explicit workspace
selection remain separate consumers and unchanged contracts.

| Requirement | Design sections |
| --- | --- |
| `REQ-WORKSPACES-DELETION-002` | Accepted publication; compatibility; failure behavior; verification and mobile |

## Components and verified diagnostic baseline

`WorkspaceEditPage` in `apps/web/app/settings/workspace/[id]/page.tsx` renders
`WorkspaceEditClient`, which resolves the target in its provider's catalogue
and mounts the keyed `WorkspaceEditForm`. `useWorkspaceEditForm` already owns
`useAppStoreApi` for settings Save. At the diagnostic baseline, it also passes
subscribed `workspaces.items` and `setWorkspaces` into `useWorkspaceDeleteDraft`.
The async `handleDelete` checks the exact current form name, awaits
`useRequest(deleteWorkspaceAction)`, then filters the array captured before the
await. Registered notifications may have changed the catalogue in that interval.
Publishing the captured array loses additions and restores unrelated removals
or stale descriptors.

`deleteWorkspaceAction` in `apps/web/app/actions/workspaces.ts` sends
`{ confirm_name: confirmName }` through `fetchJson`. The `office` feature decides
between `DELETE /api/v1/office/workspaces/:id` and
`DELETE /api/v1/workspaces/:id`. The callback bypasses the existing navigation
blocker for its accepted `router.push("/settings/workspaces")`. These request
and navigation operations stay intact.

`registerWorkspacesHandlers` in `lib/ws/handlers/workspaces.ts` independently
upserts creation, projects updates, and removes deletion using its owning store.
`AppSidebarWorkspacePicker` renders that catalogue through
`WorkspacePickerContent`. Phone `components/navigation/app-nav-sheet.tsx` uses
the same picker outside its menu scroller.

## Accepted publication

The initiating form supplies its existing `useAppStoreApi` to
`useWorkspaceDeleteDraft` instead of captured array/setter arguments. The unused
form catalogue/setter subscriptions are removed. The hook and handler remain
in `workspace-edit-client.tsx`; no new updater or store action is introduced.

Only after successful `deleteRequest.run`, read the owning provider's
`storeApi.getState()`. Filter that current `workspaces.items` by the submitted
target ID and invoke that state's existing `setWorkspaces`. The read, filter,
and setter run synchronously with no intervening await or scheduled callback.
Every nonmatching row is retained without rebuilding its descriptor, merging
an old row, or changing relative order. If the target is already absent, it
stays absent. This is a local identity removal, with no timestamp arbitration
or new delete/recreate incarnation policy.

Retain `setWorkspaces` from `lib/state/slices/workspace/workspace-slice.ts`.
It replaces items, preserves non-null `activeId`, and selects the first item
only when identity is null and the list is nonempty; it never increments
`activeIdRevision`. Do not capture identity/revision at request start or invoke
`setActiveWorkspace` on acceptance. In particular, this correction does not
repair an active ID still naming the removed target. A real `workspace.deleted`
event owns its existing fallback and revision change; acceptance must preserve
those values if that event has already applied.

The provider context is authoritative. Do not read a singleton, another root's
store, the form's local workspace snapshot as a catalogue, or a fresh HTTP list.
Use the existing API type or `ReturnType<typeof useAppStoreApi>` for immediate
glue rather than introducing a reusable publication framework.

## Compatibility and failure behavior

| Boundary | Disposition |
| --- | --- |
| `useWorkspaceDeleteDraft` / immediate form caller | Sole production ownership; current-store target filter after successful await |
| `DeleteWorkspaceCard` / exact-name guard | Existing localized dialog, eligibility, comparison and controls retained |
| `useRequest` / `deleteWorkspaceAction` / `fetchJson` | Real request, generic/Office path choice, payload and error unchanged |
| `SettingsSaveProvider` / navigation guard / client router | Real provider and accepted Settings return retained; no routing repair |
| Registered workspace notifications | Independent live writers and regression stimuli; no handler edits |
| `setWorkspaces` / `setActiveWorkspace` | Existing identity, revision and fallback policies retained; no slice edits |
| Sidebar picker / phone `AppNavSheet` | Shared current catalogue consumption; no markup or composition edits |
| Save / Add Workspace / placement / Office deletion | Independent publication boundaries; no changes or replay of prior broad checks |
| Backend cascade and resource aftermath | Existing task-owned designs; no backend, persistence or lifecycle changes |

Keep the existing catch path: translated error toast, backend error message
when present, unchanged dialog/name, and no accepted publication or navigation.
Keep the manage-scope card gate and exact-name early return. Do not add loading
controls, copy, retries, logs or telemetry. No new schema, event, response shape,
authorization policy, feature flag, or persistence boundary is needed.

## Verification and mobile

The independently authored
`apps/web/app/settings/workspace/workspace-delete-catalogue.integration.test.tsx`
and colocated `workspace-delete-catalogue.test-helpers.tsx` mount the real Page,
StateProvider/createAppStore, SettingsSaveProvider, Toast/Tooltip providers,
normal client router, and AppSidebarWorkspacePicker. Exercise actual delete
controls through real `useRequest`, action and `fetchJson`; deliver external
notifications through the actual registered handlers bound to that provider.
Mock only external transport. A small observer may expose the real provider
store; do not mock product components, hooks, actions, router, store, handlers,
or UI primitives. Reject unexpected external requests.

The [work order](../../../plans/workspace-delete-catalogue-preservation/task-01-preserve-delete-catalogue.md)
maps criteria to causal tests and exact checks. Hold target DELETE, apply a
real independent creation and unrelated deletion, assert actual picker/store
membership before acknowledgement, then accept 204 and observe the normal
Settings route and current choices. Before production edits, the independent
regression must fail at those post-acknowledgement assertions; ordinary accepted
and rejected controls must pass. Prove survivor metadata and current selection
separately. Cleanup must settle and join deferred transport and asynchronous
reads, unmount providers and restore routing/global state.

Mobile parity uses the skill's pure state/data exception. Settings delete
controls, phone navigation and the shared picker keep their existing JSX,
layout, touch behavior, scroll ownership, copy, navigation and breakpoints.
The real component/provider regression exercises the shared publication path;
no new mobile Playwright test, ASCII preview, screenshot, build or broad E2E
replay is required. A rendered or routing change invalidates this classification
and must be checkpointed before scope expansion.

## Documentation and related contracts

Public documentation remains accurate: `docs/public/team-access.md` describes
owner-only workspace deletion and manage scope, while
`docs/public/tasks-and-workflows.md`, root README and screenshot catalogue
require no new operation, setting, label, wire contract or image. This restores
choices during the existing procedure. Internal specifications and the linked
two-file plan carry the correction. The existing current-provider idiom has no
meaningful new architectural alternative requiring an ADR.

- [Per-tab Settings context](per-tab-settings-context.md).
- [Settings save coordinator](../../../decisions/0046-settings-route-save-coordinator.md).
- [Active workspace cookie](../../../decisions/0023-active-workspace-cookie.md).
- [Office mode follows active workspace](../../../decisions/2026-08-15-office-mode-follows-active-workspace.md).
- [Task-owned workspace cascade](../../tasks/system-design/archive-cascade-boundary-contracts.md#complete-workspace-deletion-transaction).
