# ADR-2026-09-15-task-owned-web-preview-feedback: Persist Web Preview Feedback with the Task

**Status:** accepted
**Date:** 2026-09-15
**Area:** backend, frontend, protocol, persistence
**Amends:** `2026-09-02-task-owned-plan-comments`

## Context

The Browser panel already injects an inspector into executor-local pages. Its
annotations live in the mounted React panel, contain only DOM metadata, and can
be copied or cleared. Closing the panel loses them, and the ordinary chat
composer cannot deliver them. Native HTML-file previews use the same port-proxy
runtime but do not expose the inspector controls.

A useful interface review spans pages and often outlives one preview panel or
Agent session. The user needs to collect comments on selected text, rendered
elements, and screenshot regions, inspect the complete set, and expose it to
exactly one Agent only after pressing the normal chat Send action.

Task plan comments already established that pending feedback shared across
sessions cannot be owned by one browser tab or session. Preview feedback has the
same delivery boundary and additionally owns staged image bytes. The previous
plan-comment decision explicitly left every other comment source session-local,
so extending task ownership requires a separate decision.

## Decision

Kandev persists one pending web-preview-feedback collection per task. It is
backend-authoritative and projected into every Browser panel, native HTML-file
preview, and composer for that task. Preview navigation, panel closure, session
selection or deletion, primary changes, browser reload, and backend restart do
not change the collection.

Pending feedback is withheld from agents. The normal composer Send action
snapshots the visible item IDs and versions and targets the selected composer
session. A promptable session uses direct-message admission; a busy session uses
the existing durable queue. This feature adds no per-item Run, broadcast, or
automatic delivery path.

The backend expands the persisted rows into visible user Markdown and attaches
each screenshot as an actual PNG prompt attachment. It consumes exactly the
submitted versions and transfers screenshot ownership in the same final
transaction that accepts the direct message or queue entry. Failure preserves
the feedback and its images. The existing caller admission identity reconciles
lost responses and prevents duplicate delivery.

Screenshot bytes use the existing file-backed attachment service. A saved
screenshot is claimed to its pending task feedback until accepted delivery
transfers it to a message or queue entry. Deleting pending screenshot feedback
deletes its unsent image. Browser storage never owns durable image bytes.

The session port proxy and trusted-preview origin remain the capture boundary.
The inspected page emits bounded selection metadata to its parent, but it never
chooses task ownership or performs backend mutations. The Kandev parent owns
comment editing, image creation and review, authorization, and persistence.

This amends the plan-comment ADR only where it said plan comments were the sole
task-owned feedback. Plan comments and web-preview feedback now have distinct
task-owned collections and a shared atomic admission envelope. Diff, file,
pull-request, walkthrough, and agent-message comments retain their existing
session-scoped lifecycles.

## Consequences

- A multi-page review survives preview, session, browser, device, and backend
  lifecycle changes until the user sends or deletes it.
- Every session composer shows the same pending count, while Send still has the
  familiar meaning of addressing that selected session.
- The task repository gains a revisioned feedback collection, screenshot claim
  lifecycle, WebSocket CRUD, and exact-reference direct and queue admission.
- Direct and queued submission can atomically consume plan comments and preview
  feedback and claim ordinary and screenshot attachments in one operation.
- Screenshot feedback consumes attachment count and byte budget when sent. A
  mixed submission that exceeds existing message limits is rejected without
  consuming feedback.
- Captured page strings remain visible user data. They do not become hidden or
  higher-priority instructions.
- A stopped runtime can make the original page unavailable without making its
  stored feedback unavailable.
- Pending collection conflicts fail closed and refresh the shared snapshot.
  Kandev never silently drops a stale item from the user's typed message.
- The attachment cleanup path must recognize task-feedback claims so deletion,
  task purge, failed creation, and backend restart cannot leak screenshot files.

## Alternatives considered

1. **Keep annotations inside each mounted preview panel.** Rejected because
   navigation or closure loses work and native HTML preview remains a separate
   experience.
2. **Persist one browser-local collection per task.** Rejected because another
   device or browser cannot recover it, image bytes become browser-owned, and
   message acceptance cannot consume it atomically.
3. **Persist feedback per Agent session.** Rejected because users review the
   task before choosing a destination, session switching hides prior pages, and
   deleting a session can destroy unrelated pending intent.
4. **Copy feedback to every session.** Rejected because copies drift, new
   sessions require backfill, and one review can be delivered repeatedly.
5. **Send from the preview panel with a dedicated Run action.** Rejected because
   it creates another destination rule and makes it easier to expose a partial
   review. The established composer Send action already handles selected-session
   routing, queueing, typed text, attachments, and retry identity.
6. **Format and clear feedback in the browser after Send.** Rejected because a
   crash, stale client, queue-capacity failure, or competing send can separate
   what the agent receives from what the UI deletes.
7. **Store screenshots inline in WebSocket or browser state.** Rejected because
   large base64 snapshots inflate shared-state frames, duplicate attachment
   lifecycle work, and cannot use existing authorized content delivery.
8. **Use the Screen Capture API as the only screenshot path.** Rejected because
   it adds a browser-mediated surface picker and permission prompt to every
   region capture. Kandev already owns the proxied iframe document and can show
   a region image for confirmation before it is saved.
