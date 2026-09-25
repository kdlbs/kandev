---
id: canvases-task-entry-presentation-design
title: Task-entry canvas presentation
status: draft
system: canvases
owners:
  - canvases
created: 2026-09-21
last_updated: 2026-09-21
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-006
---

# Task-entry canvas presentation

Lifecycle hints accelerate discovery but cannot establish that discovery is
complete. They disappear on reload and can expire from the bounded hint list.
`useTaskCanvases` supplies the authorized HTTP inventory on task entry and
lifecycle refresh. Enable the existing task-page query for desktop as well as
phones, and pass its result to `useTaskCanvasLifecycleActivation`.
Refresh the inventory after WebSocket reconnection. Do not create a polling loop.

Filter each candidate by task scope, exact task/workspace identity, and current
release eligibility. Reuse the metadata rules in
`canvasLifecycleActivationDecision`, including first-release permission review.
A failed list request is not an empty authoritative inventory. Never record
presentation or remove saved panels because a fetch failed.

On desktop, wait for the current task environment and layout restoration to
settle. Recheck task, environment, and request generation before insertion.
Use the main editor group resolved by the existing layout helpers. Add all
eligible unseen canvases in creation-time order, with canvas ID as a tie-breaker.
Focus only the last added panel once per reconciliation batch. An existing
panel counts as presented without changing its placement or active state.

Keep presentation receipts in host-owned `sessionStorage`, keyed by user,
workspace, task, canvas, and a stable tab identity. Reuse the PR-panel
offered-marker pattern from `lib/local-storage.ts`. Detect a duplicated
`sessionStorage` namespace through the live page owner in `localStorage` and
rotate the tab identity before reading receipts. Store receipts only after
successful insertion or confirmed mobile route entry. Manual opening and
restored panels also record receipts. A receipt suppresses reopening after
close and survives reload in the same tab. A new tab can offer the canvas
again. Release IDs are not part of receipt identity. No database migration or
cross-device preference is needed. Storage errors use a tab-memory fallback
and never block manual access.

On phones, select the same deterministic last candidate and navigate once to
`canvasHref`. Record the other eligible candidates as offered only after that
navigation succeeds. The existing picker exposes them without repeated redirects
on Back. Reuse `task-layout.tsx` and `CanvasHostFrame` for the focused route,
safe areas, scrolling, and touch controls. Drafts and permission-review hosts
must retain their existing distinction.

The [recovery work package](../../../plans/canvas-runtime-entry-recovery/plan.md)
owns implementation and regression coverage for this reconciliation.

This design extends the [canvas lifecycle design](agent-authored-web-apps.md).
It implements first-presentation criteria under requirement 001 and the focused
phone route under requirement 006. Canvas metadata remains authoritative.
