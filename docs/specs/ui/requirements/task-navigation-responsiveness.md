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
