---
status: current
system: tasks
requirements:
  - REQ-TASKS-SESSION-TAB-RESTART-001
---

# Session tab restart system design

## Purpose and boundaries

Compose the existing session actions without changing their backend contracts.
The [requirements](../requirements/session-tab-restart.md) belong to Tasks.
The shared UI primitives retain menu geometry, keyboard handling, and focus.

## Requirement mapping

| Criteria for REQ-TASKS-SESSION-TAB-RESTART-001 | Design section |
| --- | --- |
| AC-TASKS-SESSION-TAB-RESTART-001.1, AC-TASKS-SESSION-TAB-RESTART-001.5 | Menu integration |
| AC-TASKS-SESSION-TAB-RESTART-001.2 | Action sequence |
| AC-TASKS-SESSION-TAB-RESTART-001.3 | Failure and recovery |
| AC-TASKS-SESSION-TAB-RESTART-001.4 | Pending state and feedback |
| AC-TASKS-SESSION-TAB-RESTART-001.6 | Composer submission guard |

## Current implementation

`useSessionActions` in `apps/web/hooks/domains/session/use-session-actions.ts`
provides Stop and Resume. `isSessionStoppable` identifies active states.
`useWsAction` owns request feedback and returns a success boolean.

`wsStopSession` in `apps/backend/internal/orchestrator/handlers/handlers.go`
calls `Service.StopSession`. The executor records cancellation and schedules
runtime teardown. Its acknowledgment does not prove process exit.
`Executor.resumeSession` in `executor/executor_resume.go` invokes
`CleanupStaleExecutionBySessionID` before a terminal session resumes.
Retain this ordering and the existing execution ownership guards.

## Action sequence

`useSessionActions` delegates `restart` and its reactive pending indicator to
`useSessionRestart` in `hooks/domains/session/use-session-restart.ts`.
Reuse `isSessionStoppable` from `session-action-state.ts` as restart eligibility. Check the latest known
target and state before dispatch. Capture task and session identities before
the first await. A later navigation must not retarget the operation.

1. Acquire the browser's pending entry for the captured target.
2. Send `session.stop` with `{ session_id }` and the existing 15-second timeout.
3. After a successful response, send `session.launch` with
   `{ task_id, intent: "resume", session_id }` and the existing 30-second timeout.
4. Release the pending entry in `finally` and report the request outcome.

`useSessionRestart` owns one feedback sequence around both requests. Do not
call two independently toasting actions. Do not add fixed delays or a polling loop.

The launch request carries no prompt, replacement intent, profile change,
model override, or permission override. Existing launch authorization,
archive restrictions, admission, and recovery policy remain authoritative.
`maybePromotePrimary` in `event_handlers_streaming.go` can promote an active
sibling when Stop cancels the current primary. Keep this existing Stop behavior.
Restart does not send `session.set_primary` to reverse that promotion.

Capture whether the target is selected, its pin state, and the user navigation revision
before Stop. After the attempt settles, restore that target only if the user
still views the same task and the navigation revision is unchanged. Automatic
sibling adoption does not advance this revision. A later explicit selection does.
Restore selection after a failed Resume too, so the target's recovery stays visible.
Restore automatic selection with `setActiveSessionAuto` so it does not create a
user pin or advance the navigation revision. Restore an existing pin with
`setActiveSession`. Keep the task, target, and navigation revision guards in both cases.
WebSocket projections retain responsibility for runtime state and readiness.
The action does not write an optimistic RUNNING state or claim readiness from
the launch acknowledgment alone.

## Pending state and feedback

A WeakMap associates each application store with a transient Zustand Set of
pending session UUIDs. Session UUIDs identify the captured task/session pair.
This supports multiple hook instances, synchronous duplicate rejection,
reactive reads, and final cleanup without modifying durable session state.
Do not use the durable `pending_action` field, which represents agent input.

The model lives in `lib/session/session-restart-state.ts`, without React
dependencies. The action, composer, and WebSocket handlers share the same store
owner. During Restart, `task.updated` primary-follow and `session.state_changed`
adoption retain the currently selected pending target before clearing its pin.
State and primary projections still update normally. Both event paths check the
guard, so their delivery order cannot expose a sibling composer implicitly.
A later manual session selection remains allowed and uses its own input state.

Menus disable Stop, Resume, Delete, and Restart for the pending target.
The delete-mode tab close button is also unavailable while restart is pending.
Hide-panel close behavior remains available because it does not delete the session.
The action guard also rejects duplicate programmatic activation. Preserve the
captured operation when a menu closes or its component unmounts.
Changing session or responsive layout must not transfer the pending indicator.
Cross-browser serialization remains the existing backend lifecycle behavior.

Use one localized loading toast and update it once with success or failure.
Use wording that reports the request outcome, such as "Session restart requested".
The existing session state indicates when the agent becomes ready.
Ensure progress has a live announcement. Keep the Restart session accessible
name stable when its pending menu item is visible.

## Composer submission guard

Expose a reactive read and a synchronous query of the same store-scoped pending
set. `ChatInputArea` passes the reactive value to the shared composer. Disable
submission while retaining the editor and its draft. Show the existing localized
`sessionActionRestart` status above the composer on desktop and phone.

`useSubmitHandler` checks the current pending set before it sends, queues, or
consumes context. This check also rejects callbacks captured before Restart
began. Return `false` so rejected submission preserves the draft and attachments.
Do not rely on a disabled button alone.

During the pending sequence, the temporary CANCELLED projection must not replace
the editor with the ordinary stopped-session surface. Once the requests settle,
normal startup queue admission and recovery presentation apply again. A launch
acknowledgment alone does not prove agent readiness. Other sessions use their own
pending entries and remain usable. No server admission contract changes.

## Menu integration

Add `handleRestart` and pending information to `SessionTabMenuActions` in
`components/task/session-tab-menu.tsx`. Place Restart session directly after
Stop. Forward the action from `session-tab.tsx` and `preview-session-tab-menu.tsx`.
Use the shared menu for both workbench and preview tabs.

Extend `SessionActionsMenu` in `mobile/mobile-sessions-section.tsx` with the
same action and eligibility. Its nearest exemplar is the shipped Sessions
picker, using `MobilePickerSheet` and the row's visible overflow button.
This short secondary action belongs in the existing menu. It needs no new
navigation or confirmation surface.

Use the existing neutral text menu style and separators. No new icon, shadow,
color, or animation is needed. Preserve the established keyboard and focus
behavior. Keep fine-pointer density unchanged. Scope 44px hit areas to phone
and coarse-pointer controls, including 640-767px phone widths.

The picker retains its fixed header and internal scroll region. The menu
retains its responsive bottom-sheet treatment, safe-area clearance, and
contained scrolling. Test the new action at phone width, a narrow fine-pointer
width, and the 768px layout boundary. Do not change desktop layout preferences.

All new copy belongs in `task` locale catalogs. Supply English, Portuguese,
Simplified Chinese, Japanese, and Korean. Generate both Traditional Chinese
catalogs and refresh pseudo through repository scripts.

## Failure and recovery

Stop rejection ends the sequence. Resume rejection ends the sequence and
leaves ordinary Resume or existing recovery controls available as projected.
Stopping an in-flight durable prompt can require explicit recovery because its
outcome is uncertain. Restart must preserve that safety boundary and must not
replay the prompt to make Resume succeed.
Do not roll back a completed Stop by claiming that the agent remains running.
Report a missing connection explicitly rather than returning silent success.

A timeout has an uncertain outcome. Stop the client sequence, report the error,
and use normal session projections before another user attempt. Do not replay
the request automatically. Missing or archived targets remain subject to the
existing server checks. No failure deletes conversation or workspace data.

## Persistence and observability

No schema, server API, feature flag, or durable restart record is added.
Existing stop and launch logs remain the diagnostic source. Frontend feedback
covers the combined action. No new metric is required.

## Related contracts

- [Opening resumes the selected conversation](../../../decisions/2026-09-18-session-open-resumes-conversation.md)
- [Explicit recovery settings](../../../decisions/2026-09-29-explicit-resume-settings.md)
- [Configuration Chat restart](configuration-chat-restart.md)
- [Implementation plan](../../../plans/session-tab-restart/plan.md)
