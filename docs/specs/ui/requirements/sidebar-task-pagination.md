---
status: active
system: ui
created: 2026-09-26
owners:
  - Kandev
---

# Sidebar Task Pagination

## Overview

Pagination depends on task-list size, not archive status or saved-view identity.
This requirement retains its original stable ID after expanding from archived views.
Archive membership remains defined by [archived views](sidebar-archived-filter.md).

## Requirements

### REQ-UI-SIDEBAR-ARCHIVED-FILTER-002: Bounded sidebar task browsing

**Intent:** Every sidebar view uses the same size-based pagination rule.

#### Acceptance criteria

- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.1:** Every built-in, saved, and draft sidebar view shall load at most 100 task rows initially. It shall not automatically traverse subsequent pages.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.2:** Previous and Next controls shall appear only when the filtered list contains more than 100 displayable task rows. Group headings and continuation labels shall not count as tasks. Only the current page shall be rendered as the sidebar listing; a bounded set of recently visited first pages may be retained for view switching, without accumulating subsequent pages.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.3:** All saved-view filters, sorting, grouping, pin precedence, and manual child order shall apply before page boundaries. A later matching task shall remain discoverable through filtering.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.4:** Last activity shall rank complete included task trees, including descendants outside the current page. Each task shall show its own activity time.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.5:** Page boundaries shall retain group and parent context. Paging shall not change task relationships or silently promote a child to a root.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.6:** Foreground refresh shall refresh only the current page. Failed refreshes shall retain its contents and offer Retry.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.7:** View, filter, sort, grouping, or collapse changes shall reset to the first page. Late responses shall not overwrite another query or workspace.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.8:** Live task changes shall update visible rows and invalidate affected page membership. Settled reads shall converge without duplicates or a browser reload.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.9:** Phone navigation shall offer the same paging and conversation access. Controls shall remain touch-reachable, with contained scrolling and no document horizontal overflow.

- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.10:** Text ordering shall remain consistent across pages and across the 100-task threshold on the same server, independent of browser locale.

- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.11:** Successful paging shall replace the list and scroll only its list area to the top. The open task and conversation shall remain unchanged.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.12:** A failed page read shall retain the displayed page, scroll position, and open conversation, with Retry available.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.13:** Active-task actions shall preserve their existing meaning across page boundaries. A partial page shall never be treated as the complete task inventory.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.14:** Repository and workflow membership filters shall accept at least 1,000 selected values per clause within the overall query limit. An individual value's size limit shall not apply to the combined selection. Existing saved views shall load without editing when their values satisfy these limits; unsupported or excessive selections shall fail explicitly without truncation or broadening.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.15:** Returning to a recently visited view with an eligible retained first page shall show that page before its refresh response arrives, on desktop and phone. The view shall refresh in the background without blanking its rows, shifting them to display refresh status, or changing the open conversation. An unvisited or expired view without complete shared task data shall show a loading state, never another view's rows, except for the same-view disclosure continuity defined in 002.25.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.16:** Recently visited results shall be finite, expire, and never survive an account or workspace context change. Known deleted or no-longer-matching tasks shall not reappear from retained results. Delayed responses shall not repopulate an invalidated context.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.17:** Each task-list surface shall show at most one query-error announcement. A rejected filter shall identify the affected filter and the actionable limit or correction. Transport and server failures shall offer Retry without describing an active-task view as an archive failure. Errors shall remain distinct from successful empty results.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.18:** A failed background refresh shall retain usable rows and identify them as not refreshed, with Retry available. Access denial shall clear the affected retained results. Loading, refreshing, and recovery controls shall remain localized, keyboard accessible, and touch reachable.

- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.19:** For a fixed workspace of at most 101 tasks, 100 repeated reads shall add at most 512 MiB of backend resident memory. Measure above an initialized baseline with up to four concurrent requests and no unrelated workload. This applies to every supported sort and group, including Last activity with State grouping. Reads shall preserve the selected view and existing page semantics.

- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.20:** Homepage and sidebar task overviews shall reuse one accepted task record per task in the current account and workspace. Accepted live updates shall appear on both surfaces without another sidebar read for unchanged view membership. During a pending sidebar read, repeated live updates shall retain the newest accepted task values even when updates arrive out of order and the task was not previously loaded, including archived tasks. An older response shall not undo those values, and a newer response shall still be accepted. Task mutation time and status-summary revision shall resolve their own fields independently; omitted fields shall not clear accepted values.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.21:** When complete, current task data and supported ordering semantics cover a view, its first page shall render without waiting for a sidebar request. The sidebar shall not fetch those tasks again merely because it mounted. Local results shall preserve server filtering, complete-tree ordering, counts, and page boundaries. Existing complete data can supply subsequent pages locally.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.22:** When coverage is incomplete, stale, or unavailable, the sidebar shall fetch only the requested bounded view page. Archived saved views shall fetch on demand unless complete eligible archived coverage already exists. A partial collection shall never establish an authoritative empty result, total, or complete-tree order.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.23:** For the same authorized view and workspace, ordinary task updates during a successful read shall not indefinitely prevent initial rows from appearing. The sidebar shall reconcile known newer changes, display safe results, and refresh in the background. Superseded views, account changes, access denial, and known deletions shall retain their stale-response safeguards.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.24:** Repeated page and view navigation shall not retain every task encountered. Shared records shall remain only while an active consumer or a bounded reusable page owns them. Cached records shall not count as proof of complete view membership.

- **AC-UI-SIDEBAR-ARCHIVED-FILTER-002.25:** Changing only group or subtask collapse state in the same authorized view shall apply the disclosure immediately and keep eligible already displayed rows and headings visible while the bounded replacement page loads, on desktop and phone. It shall not replace the list with task-loading placeholders. Expanding shall not invent unseen tasks or infer complete membership from a partial page. The replacement shall request page 1; page controls shall not navigate using obsolete bounds during the transition. A failed replacement shall retain eligible rows and offer Retry for page 1. Workspace, account, access, filter, sort, grouping, locale, or ordering-preference changes shall not reuse disclosure-only display continuity. Known removed tasks shall remain absent, superseded responses shall not settle a newer disclosure, and collapsing every group shall keep the headings visible without implying an empty task inventory.

## Counting and scope

Count task rows after filtering and collapse visibility, before paging.
Hidden descendants still participate in complete-tree ranking but do not create empty pages.
Group headers and boundary context do not consume the 100-task allowance.
For 0 through 100 displayable tasks, hide the paginator. At 101 tasks, show two pages.
A filter can reduce a large workspace to a small list without pagination controls.

This applies to desktop sidebar, phone task picker, and app-navigation task lists.
It does not add pagination to Kanban columns, the full Tasks page, or the command panel.

## Implementation plan

- [Sidebar loading and archived navigation](../../../plans/archived-sidebar-loading/plan.md)
- [Sidebar view loading repair](../../../plans/sidebar-view-loading-repair/plan.md)

- [Shared sidebar state and query memory](../../../plans/sidebar-query-memory/plan.md)

- [Sidebar disclosure continuity repair](../../../plans/sidebar-collapse-continuity/plan.md)

- [Archived sidebar update freshness repair](../../../plans/archived-sidebar-update-freshness/plan.md)
