---
id: integrated-control-navigation
title: Integrated control navigation
status: draft
system: ui
owners: [kandev]
---

# Integrated control navigation

### REQ-UI-INTEGRATED-CONTROL-001: Native navigation and original conversations

- **AC-UI-INTEGRATED-CONTROL-001.1:** OFF preserves legacy navigation and performs no integrated reads; every shipped profile defaults OFF.
- **AC-UI-INTEGRATED-CONTROL-001.2:** ON adds the picker destination, five separate menus, and the same tree of workspaces with one coordinator conversation entry per coordinator on desktop and phone.
- **AC-UI-INTEGRATED-CONTROL-001.3:** Selecting an existing conversation keeps integrated navigation, original IDs and drafts, with no prepare/open/resume/prompt or agent launch.
- **AC-UI-INTEGRATED-CONTROL-001.4:** Existing native permissions apply; unavailable and failed reads are distinct from empty data; scope limitations are not reported as completed capabilities.
- **AC-UI-INTEGRATED-CONTROL-001.5:** A coordinator without a current conversation shows a "+ Coordinator conversation" entry only to users with workspace management access. Only an explicit click on it calls the existing coordinator conversation open path (profile check, then the conversation route), refreshes the tree, and opens the new conversation; a failure says why (unusable profile, removed coordinator, conflict, or open failure) and navigates nowhere.

The workspace picker offers an opt-in Integrated control destination. It owns
a dedicated navigation context, rather than representing a synthetic workspace.
Its sidebar keeps five destinations: Overview, Activity, Workspaces,
Provider / model, and History. Available workspaces contain their coordinators
and existing current conversations. Choosing a conversation renders the original
task/session centrally while keeping this navigation on desktop and phone.

Existing workspace permissions remain authoritative. Listing uses workspace
read access; the existing interactive coordinator conversation requires workspace
management access. Navigation never creates, prepares, resumes, copies, or moves
a task/session, sends a prompt, or starts an agent. The one explicit exception is
the "+ Coordinator conversation" entry shown when a coordinator has no current
conversation: clicking it calls the existing coordinator conversation route and
then opens that conversation. Session drafts, selections,
questions, results, and histories retain their original identity.

The original task's GitHub/GitLab/Azure links, connection status, feedback,
and explicit VCS actions use its own workspace. Adding check feedback to chat
uses that conversation's session. Keeping Integrated control navigation must
neither redirect these operations to the selected regular workspace nor replace
that workspace's VCS cache.

This first slice exposes only existing workspace/coordinator/current-conversation
data. Full execution dashboards, provider/account/model usage, historical
conversation discovery, and multiple independently live conversations remain
parts of the same feature, not capabilities claimed by this slice. A coordinator's
current task, an earlier archived task, and runtime sessions of one task are
different objects. No new aggregate API, ACL, or task ledger is introduced; the
existing reads are repeated every 30 seconds while the page is visible.

`features.integratedControl` / `KANDEV_FEATURES_INTEGRATED_CONTROL` is off in
every shipped profile. Disabling it restores the existing navigation without
deleting data or changing active work. Coordinator data also requires the
existing coordinator feature. AI cross-workspace control is a separate feature;
its completion does not block this authorized browser read surface.

### REQ-UI-INTEGRATED-CONTROL-002: Office aggregate dashboard

- **AC-UI-INTEGRATED-CONTROL-002.1:** With Integrated control and Office both ON, Overview shows a stall band, an attention list, and a per-workspace comparison table fed by one `GET /api/v1/office/workspaces/aggregate` read (`REQ-OFFICE-WORKSPACE-OVERVIEW-001`). With Office OFF the aggregate is not read and these panels are absent; with Integrated control OFF nothing changes.
- **AC-UI-INTEGRATED-CONTROL-002.2:** A workspace is stalled when it has in-progress tasks and no working agent. The attention list raises one row per stalled workspace, per workspace with blocked tasks, and per workspace with pending approvals, in that order and largest count first; each row opens the Office page of its own workspace.
- **AC-UI-INTEGRATED-CONTROL-002.3:** The comparison table shows one row per Office workspace with its status and task, approval, and working-agent counts, plus a total row. Listed workspaces the aggregate does not cover are counted as not included, not as zero.
- **AC-UI-INTEGRATED-CONTROL-002.4:** History shows the aggregate's merged recent Office activity, newest first, with consecutive repeats of the same action by the same actor on the same target folded into one row with a count.
- **AC-UI-INTEGRATED-CONTROL-002.5:** Loading and failed reads are distinct from empty data. A failed refresh keeps the last snapshot and says so once, in the page banner. Reads repeat every 30 seconds while the page is visible, on foreground return, and on the Refresh button; a read in flight is never aborted for another.
- **AC-UI-INTEGRATED-CONTROL-002.6:** On phone widths the panels cause no horizontal page overflow and their links are at least 44px tall.

The second slice reads Office's read-only multi-workspace aggregate. It shows
only what that read returns. Execution grades, provider usage limits, inference
capacity, per-task stall signals (queued messages, failed sessions, repeated step
moves), and recorded control history are not part of this slice and are not
inferred from the aggregate.

### REQ-UI-INTEGRATED-CONTROL-003: Recorded Native comparison

- **AC-UI-INTEGRATED-CONTROL-003.1:** With the existing Integrated control and Office features enabled, Overview retains the Office comparison and adds separate Native task-stage and session tables for the authorized workspaces. Native reads opt into the existing workspace dashboard GET with `view=control`; its default payload and authorization remain unchanged.
- **AC-UI-INTEGRATED-CONTROL-003.2:** Current task, session and queue counts use recorded workspace-scoped, unarchived, non-ephemeral, non-automation tasks. Input-waiting counts unfinished tasks by their deterministic primary session, never all waiting sessions. Session, queued-message, completed-task and transition counts keep their own units and the anchored 24-hour window.
- **AC-UI-INTEGRATED-CONTROL-003.3:** The five display groups are work, review, held, completed and unknown. Raw workflow-step names/counts remain available through initially collapsed numeric disclosures with pointer and Enter access; no readiness or classification is guessed from names.
- **AC-UI-INTEGRATED-CONTROL-003.4:** Dashboard-only reads follow the existing authorized snapshot revision without a new timer. Identity/resource changes and denied reads clear old observations; unsupported, loading and failed reads remain distinct from a measured zero. Transient failures may retain the same authorized, dated snapshot.
- **AC-UI-INTEGRATED-CONTROL-003.5:** Both tables retain original workspace navigation, localized units, right-aligned numbers and independent horizontal scrolling. This slice introduces no execution, recovery, capacity, provider, conversation, task-list, history or ledger actions.
