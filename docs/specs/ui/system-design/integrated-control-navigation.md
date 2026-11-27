---
id: integrated-control-navigation-design
title: Integrated control navigation design
status: draft
system: ui
owners: [kandev]
requirements:
  - REQ-UI-INTEGRATED-CONTROL-001
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
Refresh is explicit, foreground-driven, or driven by existing WebSocket events;
there is no timer. Failures remain distinct from empty results, and failed or
denied collections do not retain selectable stale conversations.

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
| v Workspace A            |                                      |
|   v Coordinator A        |                                      |
|     Current conversation | Existing per-session composer        |
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
| v Workspace / Coordinator      |
|   Current conversation         |
+-------------------------------+
```

The five dashboards consume the facts available in this slice and label current
conversation rows explicitly. They do not label historical runtime sessions as
independent conversations or manufacture execution/usage measurements. Later
work adds those data sources and the independent conversation lifecycle. The
existing Office-only aggregate contribution is neither duplicated nor required.

AppNavSheet owns phone navigation scrolling; its integrated tree does not create
a second scroll region. Phone widths below 768px and coarse-pointer layouts use
at least 44px active navigation, disclosure, refresh, and session targets. Fine
desktop pointers retain the surrounding compact controls. Permanent Playwright
cases cover both desktop and phone user outcomes plus narrow fine-pointer hit
testing, containment, scroll ownership, dismissal, and unchanged selection.
