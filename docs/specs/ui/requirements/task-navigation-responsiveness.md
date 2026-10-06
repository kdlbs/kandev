---
status: active
system: ui
created: 2026-09-28
owners:
  - kandev
---

# Task Navigation Responsiveness Requirements

## Purpose and ownership

Users need to move between tasks, the overview, and task panels without losing
usable content while unrelated data loads. UI owns this reusable navigation
contract. Workspaces retain filesystem and recovery authority; task and runtime
systems retain session eligibility, execution, and authorization authority.

## Requirements

### REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001: Responsive task navigation

**Intent:** Make available task content usable during hydration and refresh,
without redundant reads or data from another navigation context.

#### Acceptance criteria

- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.1:** The overview SHALL render when
  any workflow snapshot is absent, including a mixture of loaded and unloaded
  workflows. Each available workflow SHALL retain its own tasks and visibility
  preferences while another workflow loads, arrives, or is removed.
- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.2:** Concurrent consumers of the same
  shell list, commit snapshot, or cumulative diff SHALL share an outstanding
  read for the same authorized context and inputs. Mounting another consumer
  SHALL NOT alone repeat an already satisfied initialization read. A real
  invalidation during a read SHALL cause a subsequent fresh read; a burst of
  invalidations during that read SHALL require only one follow-up.
- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3:** Returning to a recently visited
  Files panel SHALL show a retained tree when one remains available for that
  same valid workspace context, while refreshing it. Retention SHALL be bounded
  and optional: an evicted tree or a new context SHALL use normal loading
  behavior, without compromising correctness.
- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.4:** A newly available root and each
  successfully restored folder SHALL become usable without waiting for every
  expanded folder. Independent folders SHALL restore with bounded concurrency;
  descendants SHALL wait for their parents. A transient failure in one branch
  SHALL preserve other available branches and allow retry. Only authoritative
  absence SHALL remove a remembered expansion.
- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5:** Switching task, environment,
  workspace, authenticated identity, connection generation, or workspace
  restoration attempt SHALL prevent obsolete responses from changing the
  current view. Reuse SHALL never cross authorization or workspace boundaries.
  A declared unavailable workspace SHALL take precedence over retained data.
- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6:** Desktop and phone SHALL provide
  these outcomes through their existing navigation. Available file rows SHALL
  remain operable during restoration. Phone Files SHALL retain its focused
  surface, visible touch actions, safe-area clearance, and a single content
  scroll owner without horizontal document overflow.

- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.7:** Selecting a task already present
  in the current workspace SHALL render its available task context and owned
  cached conversation without waiting for route refresh requests. Client
  navigation SHALL NOT wait for unrelated optional boot enrichment. Unknown
  task/session ownership SHALL resolve before displaying a conversation;
  stale responses SHALL NOT replace the selected task or newer session state.

- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.8:** When a mounted task-session
  resolver switches from a task with a settled fallback to a different uncached
  task, including closing and reopening on that different task, it SHALL expose
  no fallback session from the previous task while the current read is pending.
  Obsolete successes and failures SHALL NOT change the current result or its
  loading state.
- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.9:** The current task's available
  session collection SHALL take precedence over a fetched fallback: its marked
  primary session, or its first session if no primary is marked, SHALL remain
  selected. An uncached task's successful fallback SHALL retain the first
  returned session behavior.
- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.10:** With no selected task, the
  resolver SHALL expose no session and no active loading indication. A current
  pending fallback read SHALL indicate loading; successful empty reads, failed
  reads, and unavailable transport SHALL settle without a fallback session or
  loading indication. These cases SHALL retain the existing result interface.
- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.11:** Independently mounted
  task-session resolvers SHALL retain independent fallback and request state.
  Switching or settling one SHALL NOT alter another's result.

- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.12:** When a mounted file-review
  reader changes session, its returned review map SHALL belong only to the
  selected session, including before deferred initialization settles. A late
  response for the previous session SHALL NOT change the current session's
  reviewed or stale classification, even for identical file identities and
  diff hashes. Returning to a session SHALL permit reuse of its own shared
  cache without reviving retired local publishers.
- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.13:** A file-review reader with no
  session SHALL return an empty map and no loading indication. Retirement on
  session change or unmount SHALL prevent obsolete deferred loading, cache-hit
  work, notifications, responses, and finalization from changing that reader.
  A current reader's newly initiated read SHALL retain loading until its own
  settlement; unavailable transport and settled cache reuse SHALL be idle.
- **AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.14:** Same-session file-review
  readers SHALL retain shared cache notifications, fetched-session reuse and
  request coalescing. Current-session success, empty and failed reads, and
  optimistic mark, unmark and reset behavior SHALL retain their existing
  semantics. Desktop and phone SHALL receive the same session-owned review
  state through their existing surfaces and result interface.

## Compatibility

These criteria supplement existing [column visibility](board-step-visibility-filter.md),
[file-tree interaction](file-tree-chat-context.md), and mobile navigation
contracts. They do not change filtering, session selection eligibility, message
pagination, file operations, or backend API semantics.

No universal millisecond target is introduced. Causal browser tests prove that
available content renders before deliberately held, unrelated responses.
Comparable isolated measurements assess actual navigation improvement.

## System design

- [Task navigation responsiveness](../system-design/task-navigation-responsiveness.md)

## Implementation plans

- [Task navigation responsiveness](../../../plans/task-navigation-responsiveness/plan.md)
- [Task session fallback ownership](../../../plans/task-session-fallback-ownership/plan.md)
- [Session file-review reader ownership](../../../plans/session-file-review-reader-ownership/plan.md)
