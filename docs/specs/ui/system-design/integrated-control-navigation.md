---
id: integrated-control-navigation-design
title: Integrated control navigation design
status: draft
system: ui
owners: [kandev]
requirements:
  - REQ-UI-INTEGRATED-CONTROL-001
  - REQ-UI-INTEGRATED-CONTROL-002
  - REQ-UI-INTEGRATED-CONTROL-003
  - REQ-UI-INTEGRATED-CONTROL-004
---

# Integrated control navigation design

The `/control` route family owns the navigation mode. Workspace selection in
the ordinary picker still updates the existing real workspace store/cookie;
entering Integrated control only navigates. Conversation targets are encoded as
original workspace/coordinator identifiers in the route. The active real
workspace is not rewritten by central selection.

A route-scoped provider shares authorized workspace/coordinator reads between
the persistent app sidebar, phone AppNavSheet, and central content. Reads use
existing native APIs and discard superseded or retired identity responses.
Reads run on entry, every 30 seconds while the page is visible, on foreground
return, on the header Refresh button, and after existing WebSocket events. One
scheduler (`control-refresh-scheduler.ts`) owns that lifetime: events settle for
2 seconds and start at most one read per 10 seconds, and any request during a
read joins one follow-up read instead of aborting the read in flight. The header
shows a fixed-width `HH:MM:SS` last-refresh time and spins only the Refresh icon.
A transient failure keeps the last values and is reported once, in a banner under
the header, with 5, 10, 20, 40 then 60 second retries; a denied read clears its
data and is not retried early. Failures remain distinct from empty results.

The sidebar tree lists each workspace with one coordinator conversation entry
per coordinator, without status, loading, or error lines of its own. An entry
with a current conversation links to it. An entry without one is an explicit
"+ Coordinator conversation" action: it calls the existing coordinator
conversation route (`runOpenSequence`, the same profile check and POST the
coordinator page uses), refreshes the snapshot, and then opens the result.

The central adapter resolves only `conversation_task_id` already returned by
the coordinator list and existing sessions of that task. It verifies the task's
workspace/origin/coordinator relationship and session ownership. Ownership results
are keyed by authenticated read generation, workspace, coordinator, and task. A
changed owner masks the previous task during render, before effects can clear
it, so the original renderer cannot commit under an unverified new workspace
claiming the same task. The adapter never calls
the coordinator conversation POST. The existing QuickChatSessionView is reused
with automatic recovery disabled and without an initial prompt or draft. Focus and visibility
recovery respect that inspection guard; changing it invalidates pending
automatic recovery responses before session resume. Existing
session draft storage remains responsible for input continuity. Terminal and
archived state is read from the original task/session; selection never makes a
replacement. The existing management permission gates interaction.

A task-bound context supplies the original workspace/session to VCS chips,
connection checks, feedback, merge/unlink operations, and add-to-chat context.
Outside an embedded task this boundary retains the existing active-workspace
defaults. GitLab's existing workspace-keyed reads are reused. GitHub's current
global association cache belongs to regular workspace navigation, so the embedded
task has one local response provider using the existing task-ID read API and
WebSocket update/deletion events. It displays only associations whose task and
workspace match the verified original task; it never republishes those rows into
the regular workspace cache. Foreground and explicit refresh reuse existing
paths without a new poller or a second execution engine. Unmount invalidates
retained callbacks and pending explicit sync continuations. A successful unlink
also invalidates reads started while the deletion was pending.

Embedded Azure PR reads likewise remain local to the authenticated original task
and workspace. They do not replace the ordinary workspace's Azure map or reuse
its module-level snapshots. Stale-association refresh uses the existing authorized
API with captured identity and abort guards, including before deriving a sync
from a list response. Ordinary Azure callers retain their existing cache owner.
Integrated workspace collection reads update only the local route projection;
they do not republish a full snapshot over the regular navigation's live list.

No new backend read capability is added: existing APIs retain their native
authorization. The backend typed runtime registry authoritatively publishes the
new navigation flag. All new routes, sidebar branches, and picker entries use
that flag; OFF performs no integrated data reads.

Desktop:

```text
+--------------------------+--------------------------------------+
| Kandev / Integrated [v]   | Integrated control         [Refresh] |
| Overview                 | Workspace / Coordinator              |
| Activity                 | Original task / session              |
| Workspaces               |                                      |
| Provider / model         | Original conversation                |
| History                  |                                      |
| Workspace A              |                                      |
|   Coordinator convers.   |                                      |
| Workspace B              | Existing per-session composer        |
|   + Coordinator convers. |                                      |
+--------------------------+--------------------------------------+
```

Phone:

```text
+-------------------------------+
| [Menu] Integrated control      |
| Workspace / Coordinator        |
| Original conversation          |
| Existing composer              |
+-------------------------------+
| Menu drawer                    |
| Integrated control [v]         |
| Overview / Activity            |
| Workspaces / Provider / History|
| Workspace                      |
|   Coordinator conversation     |
+-------------------------------+
```

The five dashboards consume the facts available in this slice and label current
conversation rows explicitly. They do not label historical runtime sessions as
independent conversations or manufacture execution/usage measurements. Later
work adds those data sources and the independent conversation lifecycle. The
Office multi-workspace aggregate is read only by the Office aggregate dashboard
section below; navigation does not depend on it.

AppNavSheet owns phone navigation scrolling; its integrated tree does not create
a second scroll region. Phone widths below 768px and coarse-pointer layouts use
at least 44px active navigation, disclosure, refresh, and session targets. Fine
desktop pointers retain the surrounding compact controls. Permanent Playwright
cases cover both desktop and phone user outcomes plus narrow fine-pointer hit
testing, containment, scroll ownership, dismissal, and unchanged selection.

## Office aggregate dashboard

`REQ-UI-INTEGRATED-CONTROL-002` adds one read to the route-scoped provider:
`useControlAggregate` calls `getWorkspaceAggregate` when the route is active,
the user is authenticated, and `features.office` is on. It keeps one snapshot
per authenticated read identity and discards responses from a retired identity.
It uses the same scheduler as the workspace snapshot: every 30 seconds while
visible, on the header Refresh button, and on foreground return, never aborting
a read in flight. It adds no WebSocket subscription. Workspace comparison stacks
two tables: task counts and pending approvals first, then working agents.
Both retain workspace links, status, fixed column widths, right-aligned tabular
numbers, dimmed zeros, and aggregate totals. Each uses the shared `@kandev/ui/table`
primitives with TanStack Table and its own horizontal scroll container. The
Office aggregate counts agents, not runtime sessions; it supplies no native
workflow-step breakdown or waiting-input session counts to infer or hide.

Aggregate failures are handled apart from workspace snapshot failures. A failed
aggregate refresh keeps the last snapshot, retries on the 5, 10, 20, 40 then 60
second schedule, and is reported in the page banner as the Office summary
failing, not inside the panels. A failed first read, with no snapshot to keep,
is reported in place of the panels, and the rest of the dashboard still renders.
The navigation flag stays `features.integratedControl`; no new flag is added.

`control-aggregate-signals.ts` derives everything from the aggregate entries as
pure functions: workspace status (stalled, blocked, active, idle), totals, the
ordered attention list, the uncovered-workspace count against the navigation's
workspace list, and the folded activity groups. `control-aggregate-overview.tsx`
renders the stall band, attention list, and comparison table on Overview;
`control-grouped-history.tsx` renders folded activity on History with the shared
Office `ActivityRow`. Links target `/office`, `/office/tasks`, and
`/office/inbox` with the source `workspaceId`, so the existing Office pages
select the right workspace.

```text
Overview (desktop)
+--------------------------------------------------------------+
| 1 workspace has work in progress and no working agent         |
| In progress 3   Blocked 1   Pending approvals 2   Working 1/3 |
+--------------------------------------------------------------+
| Needs attention                                               |
| [Stalled]  Alpha: 2 tasks in progress, no agent working [Open]|
| [Blocked]  Alpha: 1 blocked task                        [Open]|
| [Approvals] Beta: 2 approvals waiting for a decision    [Open]|
+--------------------------------------------------------------+
| Workspace comparison                                          |
| Tasks                                                        |
| Workspace  Status  Tasks Open InProg Blocked Done Approvals    |
| Alpha      Stalled   3    0    2      1      0    0            |
| Beta       Active    1    0    1      0      0    2            |
| Total                4    0    3      1      0    2            |
+--------------------------------------------------------------+
| Working agents                                               |
| Workspace  Status  Working agents                            |
| Alpha      Stalled   0                                       |
| Beta       Active    1                                       |
| Total                1                                       |
+--------------------------------------------------------------+
```

On phones the band facts wrap to two columns, attention rows wrap, and the
comparison tables scroll inside their own containers so the page does not overflow.
The shared activity Run link keeps a 44px hit area below 768px and with coarse
pointers, and retains a compact 28px minimum for fine-pointer desktop layouts.

## Recorded Native comparison

`REQ-UI-INTEGRATED-CONTROL-003` reuses the existing authorized
`GET /api/v1/office/workspaces/:wsId/dashboard?view=control`. The default GET
still serves the Office dashboard. A dashboard-only repository interface reads
`ControlMetrics` through the existing read pool; no route or permission changes.
Current-state predicates exclude archived, ephemeral and automation-origin tasks.
Historical transitions retain their original task scope and anchored window. The
primary-session subquery orders by `started_at DESC,id DESC`, so input-waiting
tasks count once even when several sessions wait. Recorded state/step groups
preserve custom step identities; missing step classification is not inferred.
Queue messages and inclusive, anchored 24-hour transitions retain their own units.
Completion counts distinct tasks entering a `complete_task_on_enter` step.

`NativeDashboard`, `nativeDashboard` and `readNativeDashboard` carry only these
recorded fields. The parser rejects a foreign workspace, invalid window/counts
or an ordinary Office payload. `useNativeDashboards` follows the existing
route-owned `control.data` revision and authenticated read identity; it adds no
timer, task/history/ledger read or provider observation. Retained rows are dated
and shown only for the same resource/identity; denial and contract mismatch
clear them. Failure remains visible beside the Native comparison.

`ControlNativeComparison` mounts only on the existing feature-gated Overview,
alongside the unchanged Office comparison. The task table has the original five
display groups and input-waiting tasks, completed tasks and moves. The session
table has running/failed sessions and unconsumed messages. Raw step names/counts
remain in native `details` disclosures; opening them does not navigate the row.
The existing workspace/coordinator display remains the owner of coordinator
information; this slice does not infer conversation state or execution health.

## Recorded waiting reasons

`REQ-UI-INTEGRATED-CONTROL-004` reuses the rows `useNativeDashboards` already returned for `REQ-UI-INTEGRATED-CONTROL-003`; it adds no read. `control-wait-reasons.ts` derives four task counts: `waiting_input_tasks`; recorded `BLOCKED` state from `task_stages` (falling back to the state histogram); `stage_type=approval`; and `stage_type=hold`. Terminal states (`COMPLETED`, `CANCELLED`) and `FAILED` are excluded from the stage reasons, and `BLOCKED` takes precedence over the stage reasons, so one task is not counted twice across them. An invalid recorded count makes that reason unknown instead of a partial sum. Input wait and the stage reasons stay independent, so the panel shows no total. Missing stage groups make approval/hold unknown, never zero. `control-wait-reasons-panel.tsx` mounts after the two comparison tables and before the read-state lines, links each workspace to its control workspace view, uses a two-column phone wrap with 44px links, and labels every figure through `sidebar:controlSummary.waitReasons.*`. No capacity or slot wait is inferred; it has no recorded read here.
