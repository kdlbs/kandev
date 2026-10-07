---
status: draft
system: ui
created: 2026-09-29
owners:
  - kandev
---

# Session Refresh Efficiency Requirements

## Overview

Task navigation must reveal available destination content promptly. An open
task session must stay current without repeatedly rebuilding its visible
content when the authoritative session and executor environment have not
changed. UI owns the client read and render behavior across the desktop and
phone task surfaces. Platform retains session lifecycle, subscription, and
authorization authority. Executors retain environment status authority.

## Requirements

### REQ-UI-SESSION-REFRESH-EFFICIENCY-001: Efficient session reconciliation

**Intent:** Preserve live session recovery while avoiding redundant transfer
and rendering for an unchanged session.

#### Acceptance criteria

- **AC-UI-SESSION-REFRESH-EFFICIENCY-001.1:** When a busy session is unchanged
  between reconciliation reads, the server shall return no session body for a
  valid conditional read. The client shall retain the existing session object
  and visible content without a redundant render.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-001.2:** When the authoritative session
  changes, the next successful reconciliation read shall publish the full
  updated session. A stale read shall not overwrite a newer live update.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-001.3:** Initial entry, reconnect, a
  failed read, and a server that does not support conditional responses shall
  continue to reconcile session state. A failed read shall retain the last
  usable view and allow the existing bounded retry behavior.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-001.4:** Desktop and phone task sessions
  shall display the same state, pending action, and recovery result as before
  this optimization.

### REQ-UI-SESSION-REFRESH-EFFICIENCY-002: Shared environment status refresh

**Intent:** Concurrent task-surface consumers need one current environment
status without issuing duplicate reads for the same task.

#### Acceptance criteria

- **AC-UI-SESSION-REFRESH-EFFICIENCY-002.1:** While multiple mounted
  consumers observe one task environment, they shall share one outstanding
  status read and one polling schedule for that task. An additional consumer
  shall not itself trigger a duplicate read.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-002.2:** The shared refresh shall use the
  fastest cadence required by its current consumers. Unmounting the last
  consumer shall stop its polling. A task change shall prevent the previous
  task's result from appearing in the new task.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-002.3:** Manual refresh, temporary read
  failure, and environment reset shall retain the existing status and recovery
  behavior. Desktop and phone shall continue to expose the same status and
  actions through their current surfaces.

### REQ-UI-SESSION-REFRESH-EFFICIENCY-003: Progressive task navigation

**Intent:** A task switch must reveal the authorized destination without waiting for unrelated enrichment.

#### Acceptance criteria

- **AC-UI-SESSION-REFRESH-EFFICIENCY-003.1:** After task identity and owned-session selection resolve, client navigation shall reveal the destination shell while optional resources remain pending. It shall not wait for agents, settings, workflow lists, repository enrichment, terminals, or PR feedback.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-003.2:** Task identity reads from route and surface consumers shall share one outstanding request within the same store, authentication scope, and task. Switching A to B to A shall reject obsolete results.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-003.3:** The destination shall retain correct session selection, drafts, model state, message ordering, and recovery. Pending required data shall keep dependent actions disabled. Missing or unauthorized tasks shall retain the current recovery behavior.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-003.4:** Desktop and phone shall retain their existing navigation, focus, and scroll behavior. A held optional request shall not prevent selecting another task or reading available destination content.

### REQ-UI-SESSION-REFRESH-EFFICIENCY-004: Bounded task-switch rendering

**Intent:** Task changes must not rebuild unrelated UI or repeatedly initialize expensive editors.

#### Acceptance criteria

- **AC-UI-SESSION-REFRESH-EFFICIENCY-004.1:** Unchanged task rows and contributions shall retain render identity during unrelated session publications. A mounted editor shall not be recreated for an unchanged owner and configuration.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-004.2:** Closed dialogs and inactive surfaces shall not initialize expensive editors solely because another task was selected. Opening a surface shall preserve its content, focus, keyboard, and touch behavior.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-004.3:** Repeated switches shall release obsolete editor instances, listeners, subscriptions, and timers. Draft restoration and subsequent typing shall remain correct on desktop and phone.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-004.4:** When a send is accepted after
  its composer visit has ended, its completion shall preserve the currently
  displayed draft and the saved text, rich content, and attachments of both
  sessions. This applies even when the drafts have identical text, when the
  operator returns to the original session, and when a replacement composer
  mounts for the same session. A retained submit or accepted-payload callback
  from an ended visit shall not send or clear a successor draft. Preservation
  shall remain observable after remount on desktop, tablet, and phone.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-004.5:** During an unchanged composer
  visit, an accepted send shall continue to clear its matching text and rich
  content. Newer text shall survive. If the attachment snapshot changed while
  that send was pending, the matching submitted text shall clear while the
  current attachments remain. A failed or rejected send shall preserve the
  draft. Owner checks shall not change outgoing content or submission readiness.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-004.6:** Sending from one composer
  shall not clear an independent composer's draft for a different session,
  including a composer mounted under another application store. Retiring a
  composer shall not cancel or replay an already submitted message; it shall
  only remove that visit's authority to clear drafts.

### REQ-UI-SESSION-REFRESH-EFFICIENCY-005: Shared pull-request feedback reads

**Intent:** PR consumers must share feedback work while preserving freshness and workspace isolation.

#### Acceptance criteria

- **AC-UI-SESSION-REFRESH-EFFICIENCY-005.1:** PR detail, status, and popover consumers for the same authorized workspace and PR shall share one outstanding feedback read.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-005.2:** A real invalidation during an outstanding read shall schedule one subsequent refresh. Manual refresh and mutation completion shall preserve freshness; a failed refresh shall retain usable same-PR feedback.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-005.3:** Changing PR, workspace, or authentication scope shall not display or publish feedback from the previous scope. Slow feedback shall not block task navigation or the transcript.

### REQ-UI-SESSION-REFRESH-EFFICIENCY-006: Shared settings and integration initialization

**Intent:** Concurrent task-surface consumers must not multiply shared initialization and health reads.

#### Acceptance criteria

- **AC-UI-SESSION-REFRESH-EFFICIENCY-006.1:** Agent-list consumers and route enrichment shall share outstanding reads within a store and authentication scope. Discovery retries and profile-version changes shall retain their existing freshness behavior.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-006.2:** Integration health consumers for the same provider, workspace, and authentication scope shall share one read schedule. A successful no-config response shall be a settled result until refresh or invalidation.
- **AC-UI-SESSION-REFRESH-EFFICIENCY-006.3:** Disabled integrations shall not gain new probes. Last-consumer cleanup, credential changes, explicit refresh, workspace changes, and errors shall retain current availability semantics.

## Out of scope

- Changing session state transitions, WebSocket publication, or subscription
  acknowledgement semantics.
- Changing the full session response shape, persisted data, or executor status
  authority.
- Redesigning task panels, mobile navigation, or environment controls.

## Related contracts

- [Session subscription recovery](../../platform/requirements/session-subscription-recovery.md)
  keeps the bounded state snapshot as a recovery path.
- [Task navigation responsiveness](task-navigation-responsiveness.md) covers
  read sharing during task navigation. This package extends that work with progressive route hydration and shared
  feedback/settings reads. Existing file-tree and shell-read guarantees remain.

## System design

- [Session refresh efficiency](../system-design/session-refresh-efficiency.md)

## Implementation plan

- [Session refresh efficiency](../../../plans/session-refresh-efficiency/plan.md)
- [Composer draft settlement ownership](../../../plans/composer-draft-settlement-ownership/plan.md)
  implements the draft-preservation clarification in `004.4`–`004.6`.
