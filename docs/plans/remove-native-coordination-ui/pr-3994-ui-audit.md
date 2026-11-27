# PR #3994 UI audit

## Evidence and scope

Source: [PR #3994](https://github.com/kdlbs/kandev/pull/3994),
`f24a785cd301`, merged September 27, 2026. Audit baseline:
`adc5d67f55d19dd76161eaec5ae725e20a345acb`.

The audit compares the merge commit with its parent, then checks current consumers.
It covers rendering changes, navigation, layout, confirmations, and public Host UI
exports. It is a source audit, not a rendered-browser verification or general code
review. No subagents or external plugin repositories were used.

The PR changed 462 files overall. The frontend inventory below includes shared UI,
SDK contracts, tests, localization, and support code; not every file is a visible
change. There were no `apps/desktop` changes in the commit.

## Findings and proposed disposition

| Surface | What #3994 changed | Exposure | Disposition |
| --- | --- | --- | --- |
| Native task details | Added Manager and Completion requirements rows plus history, takeover/release, criteria editing, evidence, and reasoned override dialogs | Every ordinary task detail, even with no configured claim or criteria | Remove in this package |
| Phone task details | Added the compact two-control toolbar; removed conditional top-header offset and made panel top offset zero | Every phone task detail | Remove toolbar and restore offset behavior in the same work order |
| Automation list | Added Import button and managed-schedule import/rebinding dialog; changed action wrapping | Workspace automation list, including users without a coordinator | Additional native product UI; strongest next removal candidate, outside this package |
| Automation editor | Added a third managed-conversation target option and destination selector; changed prompt/config fields and save validation for that mode | Target choice is present in the normal editor; destination selector appears for the managed mode | Additional native product UI; assess removal together with import and existing managed schedules |
| Automation run history | Added delivery-status badges | Runs with delivery status | Additional native UI; retain in this package; review alongside automation editing |
| Plugin settings | Added workspace capability approval editor, effective/declared capabilities, grant/revoke state, and audit history | Plugin details for operators with a manageable workspace | Visible plugin-system UI, not merely an API. Retain here because removing it removes the grant/revoke path; report as an explicit scope decision |
| Host reusable conversation UI | Added managed chat, composer, instance picker, queue/interactions/recovery panels, task status, and usage components | Only when a plugin invokes the corresponding `host.ui` exports | Preserve public plugin support; no unconditional coordinator navigation or task header entry |
| Retained conversation routes | `/t/:id` selects a read-only retained transcript for detached managed conversations | Detached managed conversation records, not ordinary tasks | Visible host recovery UI; retain here to preserve transcript access after uninstall |
| Task deletion and quick/config-chat cleanup | Added current confirmation IDs and deletion-preflight dependency across menus, bulk actions, sidebar, task list, and chat cleanup | Existing deletion/cleanup flows | Mostly behavioral plumbing, not added warning panels. Preserve shared authorization compatibility |
| Shared dialog primitive | Closed alert overlays no longer intercept pointer events | Every alert-dialog consumer | Small global UX fix; preserve here |
| Shared confirmation sizing | Replaced `min-h-11` with explicit `min-h-[44px]` | Task confirmation actions | Global sizing change; preserve here |
| Workflow/status projections | Added completion setting and gate summary fields; move hook accepts an override | Data plumbing, used by native and plugin callers | Preserve contract fields; do not remove as dead UI code |

The automation changes are broader native-product changes than plugin APIs alone.
A later removal package must define what happens to existing managed schedules,
imports, and read-only run history. This audit does not silently discard those data
or replace their settings with a different native screen.

Capability approval and retained transcripts are also real UI additions. Calling
them plugin support does not make them exempt from the user's original scope.
Their removal needs an explicit replacement or loss-of-function decision. This
package leaves them unchanged and reports them for review.

## Source pointers

- Task insertion: `apps/web/components/task/task-page-inner.tsx`, `TaskCoordinationControls`.
- Phone offset: `apps/web/components/task/mobile/session-mobile-layout.tsx`, `topNavHeight="0px"`; `task-layout.tsx` removed shared-task-error propagation.
- Automation list: `components/automations/automations-list-page.tsx`, `ManagedAutomationImportDialog`.
- Automation target: `components/automations/automation-editor-sections.tsx`, `TargetModeSection` and `ThenSection`.
- Run badge: `components/automations/runs-section.tsx`, `deliveryLabelKey`.
- Settings insertion: `components/settings/plugins/plugin-detail.tsx`, `PluginCapabilityApproval`.
- Plugin-only rendering: `lib/plugins/host-api.ts`, `createPluginUIApi`, exports `WorkspaceAgentChat`, `WorkspaceTaskStatus`, and `WorkspaceTaskUsage`.
- Detached routes: `apps/web/src/task-detail-route.tsx` and `apps/web/app/t/[taskId]/page.tsx`, `isDetachedManagedConversation`.
- Deletion behavior: `components/task/task-delete-confirm-dialog.tsx`, `hooks/use-task-delete-preflight.ts`, and `lib/api/domains/kanban-api.ts`.
- Shared changes: `apps/packages/ui/src/alert-dialog.tsx` and `components/task/task-confirm-dialog-shared.ts`.

Paths without an `apps/` prefix above are relative to `apps/web/`.
The task claim/gate components are private native components. They are not public
Host UI exports, so deleting their private tree does not remove a published SDK
component. API clients and domain commands remain separate.

## Existing documentation conflict

The source matches the original plan: work orders 09/10 and UI-03 explicitly
requested native rows and phone drawers. The user's correction supersedes that
presentation scope. This package updates the owning task requirement and design
and annotates the original delivery records. It does not claim the original PR
was backend-only or that its UI appeared accidentally.

`docs/public/plugins-authoring.md` currently promises native takeover and completion
confirmation controls. Task 02 corrects those promises when removal ships.

## Recovery consequence

The task header removal also removes access to its private native dialogs. Claims,
gates, audit history, and authenticated human recovery endpoints remain.
The service suite already covers stale evidence, weakening confirmation, and a
reasoned override. Claim HTTP tests cover unavailable owners and transfer/release.
No replacement plugin dialog is supplied by this package. Removing the UI must
not allow a plugin to impersonate a human or bypass a completion gate.

## Current-head caution

Current `task-page-inner.tsx` also contains later work from #3986. Apply a targeted
removal, not a checkout of the pre-#3994 file. The two offset-related layout files
have no additional diff from the audited commit at this baseline.

## Rendering-file inventory

All added or modified production `.tsx` files in the frontend/shared-UI scope
are listed below. This list includes callback/type-only changes, so it is not a
count of new screens. Remaining changed frontend files are APIs, hooks, SDK types,
locales, tests, or test/build support and are covered by the categories above.

Frontend/shared inventory: 159 paths. Production TSX inventory: 44 paths.

- `apps/packages/ui/src/alert-dialog.tsx`
- `apps/web/app/t/[taskId]/page.tsx`
- `apps/web/app/tasks/columns.tsx`
- `apps/web/app/tasks/tasks-list-view.tsx`
- `apps/web/components/automations/automation-editor-sections.tsx`
- `apps/web/components/automations/automation-editor.tsx`
- `apps/web/components/automations/automations-list-page.tsx`
- `apps/web/components/automations/managed-automation-import-dialog.tsx`
- `apps/web/components/automations/managed-conversation-destination-selector.tsx`
- `apps/web/components/automations/runs-section.tsx`
- `apps/web/components/kanban-card-menu.tsx`
- `apps/web/components/kanban/task-multi-select-toolbar.tsx`
- `apps/web/components/plugins/retained-managed-conversation-transcript.tsx`
- `apps/web/components/plugins/workspace-agent-chat-composer.tsx`
- `apps/web/components/plugins/workspace-agent-chat-instance-picker.tsx`
- `apps/web/components/plugins/workspace-agent-chat-interactions.tsx`
- `apps/web/components/plugins/workspace-agent-chat-panels.tsx`
- `apps/web/components/plugins/workspace-agent-chat-recovery-confirmation.tsx`
- `apps/web/components/plugins/workspace-agent-chat-transcript.tsx`
- `apps/web/components/plugins/workspace-agent-chat.tsx`
- `apps/web/components/plugins/workspace-task-surfaces.tsx`
- `apps/web/components/settings/plugins/plugin-capability-approval-audit.tsx`
- `apps/web/components/settings/plugins/plugin-capability-approval-editor.tsx`
- `apps/web/components/settings/plugins/plugin-capability-approval-fields.tsx`
- `apps/web/components/settings/plugins/plugin-capability-approval.tsx`
- `apps/web/components/settings/plugins/plugin-detail.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet-dialogs.tsx`
- `apps/web/components/task/sessions-dropdown.tsx`
- `apps/web/components/task/task-completion-gate-criteria.tsx`
- `apps/web/components/task/task-completion-gate-details.tsx`
- `apps/web/components/task/task-completion-gate-row.tsx`
- `apps/web/components/task/task-completion-gate-sections.tsx`
- `apps/web/components/task/task-completion-gate-surface.tsx`
- `apps/web/components/task/task-delete-confirm-dialog.tsx`
- `apps/web/components/task/task-layout.tsx`
- `apps/web/components/task/task-management-claim-details.tsx`
- `apps/web/components/task/task-management-claim-row.tsx`
- `apps/web/components/task/task-management-claim-surface.tsx`
- `apps/web/components/task/task-page-inner.tsx`
- `apps/web/components/task/task-session-sidebar-dialogs.tsx`
- `apps/web/components/task/task-session-sidebar-selection.tsx`
- `apps/web/components/task/workflow-step-disclosure.tsx`
- `apps/web/src/task-detail-route.tsx`
