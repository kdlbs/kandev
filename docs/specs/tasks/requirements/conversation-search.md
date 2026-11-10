---
status: draft
system: tasks
created: 2026-10-19
owners:
  - kandev
---

# Task conversation search requirements

## Overview

Find a task from a phrase in its conversation when its title is unknown.
Cmd+K shall search persisted user messages and assistant replies alongside
existing task metadata. Tasks owns the searchable records, access, archive
semantics, and result identity. The palette is a consumer of this contract.

This extends task discovery through a separate conversation capability. The
[workspace search contract](workspace-search.md) remains authoritative for
metadata matching and for callers outside the palette.

## Terms

- Conversation text: persisted ordinary user messages and assistant replies
  from any session belonging to an eligible task.
- Conversation match: one task with an excerpt from a matching message.
- Ready: existing history is indexed and no pending change affects that match.

## Requirements

### REQ-TASKS-CONVERSATION-SEARCH-001: Conversation-based task discovery

**Intent:** A user can locate work by what they or an assistant wrote.

- **AC-TASKS-CONVERSATION-SEARCH-001.1:** A query in the Commands or Tasks
  palette scope shall also match conversation text across the selected
  workspace's eligible tasks and all their sessions. Matching shall use a
  literal, case-insensitive substring. Existing metadata and PR/MR matching
  shall retain their behavior. Files and Contents shall retain repository
  search behavior.
- **AC-TASKS-CONVERSATION-SEARCH-001.2:** Conversation queries shall contain
  3 to 200 Unicode characters after surrounding whitespace is trimmed, with
  at least three consecutive letters or digits. Short or punctuation-only
  queries shall retain metadata results and explain the conversation query
  requirement. Operators, quotes, percent signs, underscores, and backslashes
  shall never act as search syntax. Diacritics shall remain significant.
- **AC-TASKS-CONVERSATION-SEARCH-001.3:** Results shall contain each task once.
  Non-archived tasks shall precede archived tasks. Within each archive group,
  metadata matches shall precede conversation-only matches. Commands shall
  preview at most five tasks. Tasks shall show at most twenty.
- **AC-TASKS-CONVERSATION-SEARCH-001.4:** A conversation-only match shall show
  its task title, archive or workflow state, and a bounded plain-text excerpt
  with the matched text marked. Selection shall open that task's matching
  session without starting an agent or restoring an archived task. Existing
  metadata-only result navigation shall remain unchanged.
- **AC-TASKS-CONVERSATION-SEARCH-001.5:** Desktop keyboard users and phone
  touch users shall reach the same results. Phone users shall have a labelled
  search entry without a keyboard. Results shall have touch targets of at
  least 44 pixels, one vertical scroll region, and no document overflow at
  390-pixel width. Loading, empty, indexing, limited, and unavailable states
  shall remain distinguishable and localized.

### REQ-TASKS-CONVERSATION-SEARCH-002: Correctness through record lifecycle

**Intent:** Discovery respects current ownership and does not retain removed text.

- **AC-TASKS-CONVERSATION-SEARCH-002.1:** Only users authorized to read the
  workspace and task shall receive its matches or excerpts. Existing task
  visibility exclusions shall apply. Task transfers shall immediately remove
  results from the former workspace. Deleted tasks, sessions, rolled-back
  messages, and changed-away text shall not appear in subsequent searches.
- **AC-TASKS-CONVERSATION-SEARCH-002.2:** Ordinary user and assistant message
  text shall be eligible. Tool payloads, tool output, reasoning, system events,
  plans, attachments, and repository file text shall not be included. Older
  messages with an empty redundant task ID shall resolve through their session.
- **AC-TASKS-CONVERSATION-SEARCH-002.3:** Existing conversations shall become
  searchable after upgrade without blocking application startup for a history
  scan. Until catch-up completes, the UI shall disclose incomplete coverage.
  Restart or worker failure shall resume unfinished indexing without losing
  concurrent writes. Repeated startup shall not rebuild completed history.
- **AC-TASKS-CONVERSATION-SEARCH-002.4:** On the reference workload in
  AC-TASKS-CONVERSATION-SEARCH-003.1, committed message changes shall become
  searchable within two seconds after the index has caught up. Index lag shall
  never permit stale or deleted content to appear. A failed index shall leave
  metadata search usable and disclose unavailable conversation search.
- **AC-TASKS-CONVERSATION-SEARCH-002.5:** SQLite and PostgreSQL installations
  with the supported text-index capability shall satisfy the same conversation
  semantics. A missing capability shall produce an explicit unavailable state
  and operator guidance without preventing normal task use.

### REQ-TASKS-CONVERSATION-SEARCH-003: Bounded interactive cost

**Intent:** More conversation history must not cause a scan on every keystroke.

- **AC-TASKS-CONVERSATION-SEARCH-003.1:** On a recorded 4-core, 8-GiB, local-SSD
  reference host, warm conversation queries shall have p95 server latency at
  most 150 ms for 100,000 messages and 300 ms for 1,000,000 messages.
  The corpus shall span 10,000 tasks and ten workspaces, average 1 KiB per
  message, and include selective, absent, common, Unicode, and archived matches.
  Measure each query class separately. Timed-out common queries shall disclose
  limited coverage and shall not count as successful performance samples.
- **AC-TASKS-CONVERSATION-SEARCH-003.2:** A conversation request shall return
  no more than twenty unique tasks and 240 Unicode characters per excerpt.
  It shall finish or report a bounded failure within one second on the
  reference host, excluding network transit. It shall not transfer transcripts
  or compute an exact global match count.
- **AC-TASKS-CONVERSATION-SEARCH-003.3:** Typing shall coalesce requests after
  300 ms of inactivity. Query, workspace, scope, or dialog changes shall cancel
  obsolete reads and prevent their results from replacing current results.
  Metadata matches shall remain available while conversation search runs.
- **AC-TASKS-CONVERSATION-SEARCH-003.4:** On the reference host, indexing shall
  increase p95 canonical message-write latency by no more than 10 percent or
  2 ms, whichever allowance is larger. Test ten streaming sessions at ten
  updates per second each. Historical indexing shall yield between batches,
  and shutdown shall stop and join its worker before database closure.

## Exclusions

No semantic/vector search, fuzzy conversation matching, global cross-workspace
search, unbounded result pagination, message-count totals, or new search server.
Selection opens the matching session. Exact message scrolling is separate work.
No changes to session-local Find, board filtering, mentions, or dependency search.

## Related artifacts

- [System design](../system-design/conversation-search.md)
- [Implementation plan](../../../plans/task-conversation-search/plan.md)
- [Archived results](command-panel-archived-task-results.md)
