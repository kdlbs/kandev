---
status: draft
system: platform
created: 2026-10-09
owners:
  - kandev
---

# Journey Data Loading Requirements

## Overview

Users open a board, inspect a task, and switch between its agents without loading unrelated conversation detail.
Platform owns shared loading and delivery guarantees. Tasks retain record authority. UI retains layout, saved views, and navigation semantics.

A **visible detail surface** is a chat that the user can inspect, including a visible split or preview.
A **refresh generation** is one initial hydration, reconnect, foreground refresh, or explicit invalidation of an authorized resource scope.
A **reference fixture** is the synthetic dataset recorded in the linked design. Byte budgets apply to that fixture, not arbitrary user text.

## Requirements

### REQ-PLATFORM-JOURNEY-LOADING-001: Route-scoped initial data

**Intent:** Initial loading supplies the selected journey without duplicating task records or hydrating unrelated details.

#### Acceptance criteria

- **AC-PLATFORM-JOURNEY-LOADING-001.1:** A single-board entry shall load that board's task membership without eagerly loading other boards. A multi-board entry shall load additional boards when their surfaces require them.
- **AC-PLATFORM-JOURNEY-LOADING-001.2:** A task entry shall load its selected task, compact sibling membership, selected session detail, and at most one bounded sidebar page. More unrelated tasks shall not enlarge its initial task-record set.
- **AC-PLATFORM-JOURNEY-LOADING-001.3:** Each task and session record shall appear once in the initial wire representation. Shared appearances shall retain consistent identity and status after hydration.
- **AC-PLATFORM-JOURNEY-LOADING-001.4:** A valid requested session shall be the initial selected session. An invalid, inaccessible, or foreign-task session shall never supply detail. Selection shall use the authorized primary or first eligible sibling, or no session when none exists.
- **AC-PLATFORM-JOURNEY-LOADING-001.5:** Saved filters, ordering, pagination, archived views, status precedence, and complete local coverage shall retain their existing outcomes. Partial data shall never claim complete coverage.
- **AC-PLATFORM-JOURNEY-LOADING-001.6:** On the reference fixture, task-detail boot JSON shall not exceed 512 KiB and single-board boot JSON shall not exceed 1.25 MiB before compression. Increasing unrelated tasks from 1,000 to 10,000 shall add no task or session records to either selected journey.
- **AC-PLATFORM-JOURNEY-LOADING-001.7:** Failed optional data shall not prevent authorized task detail from opening. Desktop and phone shall preserve their existing loading, empty, retry, and access-denied behavior.

### REQ-PLATFORM-JOURNEY-LOADING-002: Incremental conversation history

**Intent:** Users receive turn context for visible messages without downloading every historical turn.

#### Acceptance criteria

- **AC-PLATFORM-JOURNEY-LOADING-002.1:** Initial conversation loading shall obtain turn context only for its initial message window and current active turn. Adding older turns shall not enlarge this initial set.
- **AC-PLATFORM-JOURNEY-LOADING-002.2:** Older-page loading and search navigation shall obtain the corresponding turn context without losing message order, durations, usage, or active-turn state.
- **AC-PLATFORM-JOURNEY-LOADING-002.3:** Partial history shall never appear complete. Reconnect, concurrent completion, and interrupted page loading shall preserve newer accepted state and permit recovery.
- **AC-PLATFORM-JOURNEY-LOADING-002.4:** Successful boot history shall not immediately trigger an equivalent full-history read. Existing clients that request complete turn history shall retain their current response.

### REQ-PLATFORM-JOURNEY-LOADING-003: Shared resource loading

**Intent:** Multiple components reuse the same authorized read without creating independent refresh loops.

#### Acceptance criteria

- **AC-PLATFORM-JOURNEY-LOADING-003.1:** Concurrent consumers of the same resource and refresh generation shall share one in-flight request. Repeated invalidations during it shall schedule at most one trailing refresh.
- **AC-PLATFORM-JOURNEY-LOADING-003.2:** Successful boot data shall satisfy initial consumers without per-component repeats. A gap between boot and live delivery shall permit one shared repair. Reconnect after a gap shall still repair loaded state.
- **AC-PLATFORM-JOURNEY-LOADING-003.3:** Resource identity shall distinguish account, workspace, task, session, and response options where relevant. Obsolete work shall not populate another scope.
- **AC-PLATFORM-JOURNEY-LOADING-003.4:** Releasing one consumer shall not cancel another consumer's request. Final release or scope invalidation shall cancel obsolete work and dispose its timers.
- **AC-PLATFORM-JOURNEY-LOADING-003.5:** Temporary failure shall retain eligible existing data and bounded recovery. Failure shall not become authoritative empty data or start an unbounded retry loop.

## Related contracts

- [Interactive reads](interactive-read-availability.md), especially REQ-PLATFORM-INTERACTIVE-READS-006.
- [Task status delivery](bounded-task-status-delivery.md), especially visibility and compact navigation state.
- [Sidebar views](../../ui/requirements/sidebar-archived-filter.md) retain membership and local/server parity.
- [Task navigation](../../ui/requirements/task-navigation-responsiveness.md) retains navigation and progressive restoration.
- [System design](../system-design/journey-data-loading.md).

## Exclusions

No new layout, user setting, transcript retention limit, database pool, health bypass, or mutation-admission policy.
Quick Chat background notifications and cheaper conditional-session polling remain separate follow-up candidates.
No production latency guarantee follows from a shared-host diagnostic run.
