---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-TASK-STATUS-001
---

# Task executor status indicators

## Boundaries and mapping

REQ-EXECUTORS-TASK-STATUS-001 maps to the shared resource lifecycle (AC .1/.2),
responsive disclosure (AC .3/.4), and failure handling below. The existing
Kubernetes foundation retains API authorization and resource ownership.

## Shared resource lifecycle

Extend `apps/web/hooks/domains/session/remote-executor-status-resource.ts` and
`use-remote-executor-status.ts`. Keep the scope tuple of executor type, executor
ID, task ID and session ID, stable snapshots, in-flight deduplication, 90-second
successful-result reuse, and bounded eviction of unused entries.

The resource owns one refresh timer per actively subscribed valid scope, not one
per mounted icon. Start an eager read on subscription through the hook. Schedule
subsequent reads 90 seconds after settlement, including failures and unavailable
transport. An explicit disclosure refresh joins a pending request or starts a new
one and resets the schedule after settlement. Do not create overlapping reads.
On last unsubscribe cancel the timer. Late settlement may update its old cache
entry but must not resurrect polling. Hidden documents pause scheduled reads;
visibility restoration loads expired active scopes and reschedules fresh ones.
Manage the visibility listener only while active consumers exist. No new timer
for the authoritative external-status path or an invalid scope.

Retain `getKubernetesTaskSession` for Kubernetes and `task.session.status` for
other remote executors. A disconnected WebSocket is unavailable transport: skip
the read and retry on the existing schedule without queuing obsolete requests.
No backend schema or persistence change is required.

## Responsive disclosure

`RemoteCloudTooltip` retains the existing summary and `useTaskIconTooltipState`.
`StatusTrigger` must forward the DOM ref and all primitive-injected DOM props to
its span, preserving composed event handlers, aria attributes and positioning
attributes. Use the repository React version's supported ref pattern. Test the
real `@kandev/ui` Tooltip and Drawer rather than replacing them with fragments.
Keep Escape dismissal and hover/focus behavior consistent with task PR indicators.

Desktop retains the small glyph and bounded anchored summary. On coarse pointers,
`useTouchDrawer` selects the existing short, temporary bottom disclosure. The
nearest layout exemplar is `components/kanban/mobile-menu-sheet.tsx`: fixed header,
scrolling body, inset shape and safe-area clearance. Reuse shared facts and loading
state. Keep the compact visible glyph, but ensure its touch hit area does not
intercept adjacent task controls. The drawer owns vertical scrolling, uses dynamic
viewport bounds and restores focus; no document horizontal overflow is allowed.

## Failure and recovery

Retain old facts during refresh, publish sanitized read failures, and recover on
the next scheduled or explicit read. Unknown status must not imply ready. Existing
credential authorization and sanitization are unchanged. Do not log provider
credentials or raw errors. No new telemetry or persistence is needed.

## Verification

Use fake-clock resource tests for scheduling, duplicate consumers, hidden/visible
transitions, failure recovery, transport absence, unmount and late settlement.
Use actual primitive integration tests for DOM trigger linkage and focus/Escape.
Desktop and mobile Kubernetes E2E prove no-hover updates and real visible details.
