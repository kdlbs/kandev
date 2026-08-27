---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
created: 2026-07-31
owners:
  - nova28
---
# Automation Runs System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATION-RUNS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATION-RUNS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

Decision:
[ADR-2026-08-22-user-configured-automation-continuity](../../decisions/2026-08-22-user-configured-automation-continuity.md)

## Why

An automation that produces a report — a nightly drift sweep, a dependency audit — is read daily and edited almost never. Its output was reachable only through the automation's own edit form: Settings → workspace → Automations → open the automation → scroll past the whole configuration → expand a collapsed section → click a row. Six steps into an editing surface to perform a reading task.

The first correction lifted the runs out into a flat cross-automation feed. That was half the fix. Runs have an owner — the automation that produced them — and the question people actually ask is "how is *this* automation doing", so the primitive is the automation and its history, with the flat feed as a lens over the top rather than the front door.

## What

The automation is the object; its runs are its history. The surface is a list and a detail, not a flat feed.

**Navigation**

- The sidebar carries an **Automations** section directly under New Task, listing
  the workspace's automations by name with the same health dot the list uses. The
  list IS the navigation: picking one opens its history, not a settings form. The
  section header opens the full list, which adds the next firing and what each
  one last said — more than a sidebar row should try to hold.
- There is no separate "Runs" nav row, and the destination is named for the object
  it lists: `/automations`, matching the sidebar section and the settings page,
  with the same `IconBolt`. `/runs` still resolves so shared links keep working. Two entries pointing at the same place is
  what made the destination hard to place: the automation is the object, so the
  nav lists automations.
- The section fetches only while it is on screen — a collapsed rail or a folded
  section issues no requests, since the sidebar is mounted on every page.
- While the section is expanded in the desktop rail, `AutomationsSection` uses
  the shared 10-second live-refresh cadence to re-read
  `automation.summaries`. `useAutomationSummaries` retains its request-order
  guard, so a slower earlier response cannot overwrite a later refresh. Folding
  the section, collapsing the rail, or unmounting the desktop-only rail disables
  the cadence.
- `buildAutomationRows` remains the single health-state derivation. An idle or
  paused row keeps its existing health dot; a running row renders an animated
  loader in the same compact slot. The loader is decorative and the existing
  localized state label remains the screen-reader contract, so animation does
  not replace the semantic Running state.

**`/automations` — the agenda, not an index**

The sidebar already lists automations by name with a health dot, so this page
SHALL NOT be a second list of names. It answers the two questions neither the
sidebar nor any single automation's page can:

- **Up next** — every automation ordered by when it fires, soonest first. The
  time leads the row; the name is secondary, because the name is what the
  sidebar already gave you.
- The time is the resolved next firing (`~00:00 tomorrow · GMT+8`) or the reason
  there isn't one. The tilde is deliberate: the scheduler ticks on an interval,
  so the time is approximate and the UI SHALL NOT imply a precision it does not
  have.
- Reasons an enabled automation will not fire, each shown in place of a time: a
  run is still open and `max_concurrent_runs` is reached; no schedule is set; the
  trigger itself is switched off. An automation that will not fire stays in the
  agenda holding its reason — dropping it would make the agenda look complete
  when it is not.
- A standing constraint sits under the agenda, which is exactly what it
  qualifies: scheduled automations only fire while kandev is running.
- **Recent runs** — the cross-automation feed inline, newest first, each entry
  naming its automation. "Did anything go wrong overnight" is the other half of
  why someone opens this page; behind a link, they would have to ask for it
  twice. The filtered lens stays one click away for digging.

**Detail — `/automations/<automation-id>`**

An automation is **one conversation that recurs**. The detail view is that
conversation, not a log about it.

- The pane shows a **run's transcript**, at full size, and lands on the newest —
  "what did it say last night" is the question this page exists for, and it
  should cost zero clicks.
- A run is anchored to an exact session turn. For `new_task`, that session normally belongs only to
  the selected run. For `reuse_thread`, several run rows intentionally share one session; selecting
  a run opens the shared transcript and scrolls/focuses the selected turn instead of showing the
  latest turn for every row.
- The automation's **standing instruction** is pinned above the transcript,
  collapsed. It is the same on every run, so it belongs to the automation rather
  than to any one of them; it is what makes the turns underneath legible.
- A **composer** is always present. The run is repliable and the agent continues
  in the same session and worktree, so the reply belongs here rather than on a
  page the reader has to be sent to.
- **Runs are a rail, not the page** — a switcher over instances of the same
  conversation, grouped Running / Completed, each row a time and a state and
  nothing else. The selected run lives in the URL (`?run=<id>`): "the run that
  failed overnight" is a thing people link each other to.
- A run that produced no conversation (a skipped firing) is listed but inert.
  It has nothing to read, and offering it would render an empty pane that looks
  like a broken link.
- **Configuration is behind a `Details` link in the rail**, not a tab beside the
  transcript. A tab pair claims the two are done about equally; an automation is
  configured once and read continuously.
- A run in flight shows its live output as it happens, not a placeholder.
- A run displayed as Running moves into Completed without a manual page reload
  once its terminal result is available, including when the health summary
  temporarily reports no open runs.
- The URL is stable and bookmarkable — checking one automation daily is a first-class use, and it should cost one click from a bookmark.
- **Run now** fires the automation from the reading surface, because the alternative — waiting until tomorrow to find out whether a schedule works — is not one. A trigger the concurrency cap turns away SHALL be reported as skipped with its reason, never as a fire: nothing ran, and no run appears under Running.
- A running row exposes **Stop current run**. It cancels the exact bound turn rather than the newest
  turn on the shared task, marks the run failed, and releases the concurrency slot. On mobile it is
  a visible 44 px action in the selected-run drawer/content header, not a hover-only rail control.
- The selected transcript component is keyed by run ID and turn ID, not only by session ID. Two
  adjacent runs that share a session must still move the viewport and live-status binding to the
  selected turn.

**Entries**

- Each entry leads with **what the run said**, not with its status. Kandev's runs are prose written by an agent, so the outcome text is the deliverable; status is metadata. This is the difference between a run log and a CI job table.
- An entry shows: relative time, up to two lines of outcome, and status. In the cross-automation view it also carries the automation's name.
- An entry may show `Started new thread`, `Continued thread`, or `Replaced thread` when that context
  helps explain a continuity fallback. The safe replacement reason is detail metadata, not a run
  failure by itself.
- Outcome text is the run's `error_message` when it has one, otherwise its `summary` (the tail of the agent's last message). A failed or skipped run therefore explains itself in place. A **skipped** run is not styled as a failure: the concurrency cap turning a firing away is the cap working, and rendering the reason in red makes a correctly throttled automation look broken.
- A run that never produced a task SHALL NOT be clickable.

**Relationship to the board**

- No automation run appears on the kanban or in the task list, regardless of how it was configured (`automations-settings.md`). This surface is where every run is visible.
- The flat cross-automation feed keeps the run vocabulary and its history icon — it genuinely is a list of runs. Everything else is named for the automation.

## Data model

The feed reads `automation_runs` rows joined to `automations` for attribution. New runs persist the
conversation anchor and continuation outcome needed when several firings share one task.

| Field | Source | Note |
|---|---|---|
| all `AutomationRun` fields | `automation_runs` | including the read-time-derived `status` |
| `display_title` | `automation_runs` | trigger-specific rendered title snapshot; does not change when a shared task is reused |
| `session_id` | `automation_runs` | exact session that accepted this firing; empty when it never dispatched |
| `turn_id` | `automation_runs` | exact scheduled turn; empty for legacy, skipped, or pre-dispatch failed rows |
| `thread_action` | `automation_runs` | `created`, `resumed`, or `replaced` for a dispatched run |
| `thread_reason` | `automation_runs` | optional safe reason for replacement; never raw provider/runtime secrets |
| `automation_name` | `automations.name` | INNER join — a run whose automation is gone is unattributable |
| `summary` | `task_session_messages` | tail of the agent message for `turn_id`, truncated server-side to 280 chars |

The status shown is derived at read time, not stored: a `task_created` run whose task was deleted or whose primary session was cancelled reads as `cancelled`, and one whose task was archived reads as `archived`. Persisted terminal statuses (`failed`, `succeeded`, and `skipped`) take precedence; read-time `cancelled` or `archived` derivation applies only to an open `task_created` run. This derivation is defined once and shared with the per-automation log — the two views MUST NOT be able to disagree about the same run.

Stored `triggered` is also displayed as Running and counts as open. It exists only between durable
admission and accepted-turn binding; reconciliation turns an interrupted pre-dispatch row into a
visible failure rather than leaving it open forever. The Running filter includes both `triggered`
and `task_created`.

For new rows, the summary lookup filters by the stored turn identity and agent author. It must never
fall back to the latest message in a shared task. Rows created before turn binding existed retain the
legacy task-based projection, which still uses `idx_messages_task_author_created`.

Terminal status transitions address the run ID or exact session/turn binding. A manual reply in the
same reusable session has no AutomationRun binding and therefore cannot complete, fail, or rewrite a
scheduled run.

## API surface

Five actions. The detail view reads one automation's runs and its summary; the
list reads every automation's summary; the flat lens reads the workspace feed.

```text
automation.runs.list
  request   { automation_id: string (required), limit?: number }
  response  AutomationRun[]
```

The cross-automation action, used by the list and the flat lens:

```text
automation.runs.list_workspace
  request   { workspace_id: string (required), limit?: number }
  response  { runs: WorkspaceAutomationRun[] }
```

The health actions, one row per automation that has ever run:

```text
automation.summaries
  request   { workspace_id: string (required) }
  response  { summaries: AutomationSummary[] }   // { automation_id, open_runs, last_run? }

automation.summary
  request   { automation_id: string (required) }
  response  { summary: AutomationSummary | null }   // null = it has never run

automation.run.stop
  request   { automation_id: string (required), run_id: string (required) }
  response  { run_id: string, status: "failed" }
```

`automation.run.stop` loads the stored task/session/turn binding; the client never supplies those
identities. It succeeds only for an open run owned by `automation_id`. A terminal, missing, or
foreign run returns the same not-found result and does not cancel any newer turn.

Both facts are answered in ONE statement. Two queries are two snapshots: a run
created between them reads as a still-open `last_run` with `open_runs = 0`, so
the row renders idle and the client never starts polling — permanently stale
until someone refreshes by hand.

Every run query orders by `created_at DESC, id DESC`. Without the tie-break, two
runs written in the same second can order one way in the feed and the other way
in the summary, so a row's "what it last said" would contradict the entry
leading that automation's own activity.

The list SHALL NOT derive an automation's health from the workspace feed. That
feed is capped, so past the cap a quiet automation's newest run falls out of the
window and its row would report "No runs yet" and idle — the two claims a health
indicator must never get wrong — while the open count backing "won't fire —
still running" silently drops to zero. `open_runs` is counted by the same
definition `max_concurrent_runs` uses, so the reason shown and the cap causing it
cannot disagree. An automation with no summary row has genuinely never run.

- `limit` defaults to 50 and is capped at 200. The bound is enforced in the store, so it holds for every caller rather than only the handler.
- Runs are ordered by `created_at` descending across all of the workspace's automations.
- An empty result serializes as `[]`, never `null`.
- A missing or empty `workspace_id` returns `BAD_REQUEST`.
- `AutomationRun` responses include `display_title`, `turn_id`, `thread_action`, and optional
  `thread_reason`; clients treat empty `turn_id` as legacy or non-dispatched history.

## Permissions

The action is workspace-scoped: the caller must be authorized for `workspace_id` by the same check that gates `automation.list`. A run belonging to another workspace SHALL NOT appear in the feed, regardless of the requested limit.

## Failure modes

| Condition | Behaviour |
|---|---|
| Workspace has never run an automation | Empty state that names the next action, not a bare "no results" |
| A filter excludes everything | Distinct message from the never-run case, so the user knows a filter is on |
| Run has no `task_id` (e.g. a skipped run) | Entry renders and explains itself; it is inert, not a dead link |
| Run has no `summary` (agent never spoke) | Entry says why there is nothing to read — still running, or no report recorded. It MUST NOT fabricate an outcome, but a blank line that leaves the reader guessing is not the alternative |
| Several runs share one session | Each row shows the agent output for its own turn. Selecting another row moves to that turn even though `session_id` is unchanged. |
| A user replies between scheduled firings | The reply remains in the shared transcript but does not create or finalize an AutomationRun. |
| One run is deleted from a shared thread | Only the run row is removed. The task/session/worktree remain while another run or the automation continuation pointer references them. |
| Delete-all is confirmed | The continuation pointer is cleared first, run rows are deleted, and each distinct associated task is deleted once through normal task cleanup. |
| The list request fails | Surface the failure and offer a retry. A transient failure MUST NOT leave a permanently empty feed |
| A slow first load resolves after a later refresh | The later response wins; a stale response MUST NOT overwrite fresher data |
| An automation is enabled but cannot fire | The list row states why in place of a next-run time, rather than showing a time that will not happen |
| kandev was not running when a schedule was due | The missed firing runs once on the next tick, not once per missed occurrence. The list's standing note tells the user this can happen |
| Workspace changes while the list is open | Rows from the previous workspace are never shown under the new one, not even for one frame |
| Workspace changes while a detail page is open | The page leaves for `/automations`. An automation belongs to one workspace, so continuing to show it under a sidebar that says otherwise is a lie about where the user is |
| A run finishes while the page is open | The page stops calling it running without a reload. Polling runs only while something is open, so an idle workspace issues no repeat requests |
| A run starts after the Automations sidebar section is opened | The visible section re-reads health summaries on the live-refresh cadence and replaces the row's health dot with the running indicator without a reload |
| The Automations sidebar section is folded or the desktop rail is collapsed | Health-summary refresh stops; reopening the visible section performs an authoritative read before displaying current health |
| A visible run is still reported as open while the health summary says there are no open runs | The detail rail/drawer continues reading until the visible run receives a terminal status, then moves it to Completed without a reload |
| An open run falls outside the page's own run window | The open count comes from the server, not from the loaded window, so the page still reports work in flight and keeps polling |
| A user stops a running reused turn | The action addresses its stored run/session/turn identity, cancels that exact turn, and marks only that run failed. |

## Scenarios

- **GIVEN** a workspace with two automations, **WHEN** the user opens `/automations`, **THEN** each automation appears as one row carrying its state, its next run (or why it has none), and what it last said.
- **GIVEN** an automation whose only open run has not finished and whose `max_concurrent_runs` is 1, **WHEN** the list renders, **THEN** its row says it is paused and why, in place of a next-run time.
- **GIVEN** an automation with no cron expression set, **WHEN** the list renders, **THEN** its row says it will not run on its own.
- **GIVEN** any automation, **WHEN** the user opens its row, **THEN** the app navigates to `/automations/<automation-id>` and lands on Activity, not on configuration.
- **GIVEN** an automation with one run in flight and several finished, **WHEN** the user opens its detail, **THEN** the running one is visible without applying a filter, above the finished ones.
- **GIVEN** an automation detail is open, **WHEN** the user follows Details, **THEN** the editor appears, the runs rail stays alongside it, and the URL still identifies the same automation.
- **GIVEN** an automation with several runs, **WHEN** the detail opens, **THEN** the newest run's transcript is on screen without the user selecting anything.
- **GIVEN** a link carrying `?run=<id>`, **WHEN** it is opened, **THEN** that run's transcript is shown rather than the newest.
- **GIVEN** a requested run that is no longer in the window, **WHEN** the page loads, **THEN** it falls back to the newest rather than rendering an empty pane.
- **GIVEN** a finished run, **WHEN** the user types in the composer, **THEN** the agent continues in the same session and worktree.
- **GIVEN** three scheduled runs share one reusable session, **WHEN** the user selects the middle
  run, **THEN** the full thread remains available and the view focuses that run's exact turn and
  shows that turn's summary.
- **GIVEN** the selected and next run share a session ID, **WHEN** the user changes the run query
  parameter, **THEN** the transcript still moves to the newly selected turn.
- **GIVEN** a human reply occurs after a scheduled run completed, **WHEN** that reply turn ends,
  **THEN** the scheduled run remains unchanged and no new automation run appears.
- **GIVEN** a run is in flight, **WHEN** the user opens it, **THEN** they see the agent's output as it arrives rather than a placeholder.
- **GIVEN** a run is in flight on desktop, **WHEN** the user selects Stop current run, **THEN** the exact run becomes failed and the next firing is no longer blocked by its slot.
- **GIVEN** a run is in flight on a phone, **WHEN** the user opens its run drawer/content and taps Stop current run, **THEN** the same cancellation outcome is available without hover or horizontal scrolling.
- **GIVEN** two runs share a task but render different title-template values, **WHEN** their rows are listed, **THEN** each row shows its own persisted `display_title`.
- **GIVEN** a run whose agent asked a question, **WHEN** the user opens it, **THEN** they can reply and the agent continues.
- **GIVEN** a workspace with runs from two different automations, **WHEN** the user opens the flat lens, **THEN** all runs appear in one feed ordered newest first, each labelled with its own automation's name.
- **GIVEN** a run whose agent reported "Sweep complete across all 32 specs", **WHEN** the feed renders, **THEN** that text is visible on the entry without opening anything.
- **GIVEN** a run skipped by the concurrency cap, **WHEN** the feed renders, **THEN** the entry shows the skip reason as its outcome and status `Skipped`.
- **GIVEN** a run with a `task_id`, **WHEN** the user clicks its entry, **THEN** the app navigates to `/tasks/<task_id>`.
- **GIVEN** a run with no `task_id`, **WHEN** the user clicks its entry, **THEN** nothing navigates.
- **GIVEN** runs of mixed status, **WHEN** the user selects the `Failed` filter, **THEN** only failed runs remain listed.
- **GIVEN** runs from automations A and B, **WHEN** the user filters to automation A, **THEN** only A's runs remain and B is still offered as an option.
- **GIVEN** another workspace has newer runs than this one, **WHEN** the feed loads, **THEN** none of them appear.
- **GIVEN** a workspace with no runs at all, **WHEN** the feed loads, **THEN** the empty state invites the user to create an automation.
- **GIVEN** any automation fired, **WHEN** the user looks at the kanban or the task list, **THEN** its task does not appear in either — `/automations` is where every run is visible.
- **GIVEN** an automation run finished successfully, **WHEN** the user opens it and replies, **THEN** the agent continues in the same session and worktree rather than reporting that the session has ended.
- **GIVEN** an automation whose newest run is older than the workspace feed's cap reaches back, **WHEN** its row renders, **THEN** it still shows that run's outcome rather than "No runs yet".
- **GIVEN** a run is in flight, **WHEN** the user leaves the page open, **THEN** it stops reading "Running" once the run finishes, without a reload.
- **GIVEN** an idle automation is visible in the desktop sidebar, **WHEN** its schedule starts a run, **THEN** the same row shows the animated running indicator and localized Running state without a reload.
- **GIVEN** a running automation is visible in the desktop sidebar, **WHEN** its last open run finishes, **THEN** the same row returns to its non-running health indicator without a reload.
- **GIVEN** the detail rail or drawer shows a run as Running while the health summary reports zero open runs, **WHEN** that run receives a terminal status, **THEN** the run moves to Completed without the user reloading the page.
- **GIVEN** an automation with an open run older than its own capped run window, **WHEN** its detail page renders, **THEN** it still reports that something is running and keeps polling, because the count comes from the server rather than from the window.
- **GIVEN** the user switches to another workspace while a detail page is open, **WHEN** the switch lands, **THEN** the page leaves for `/automations` rather than showing another workspace's automation.
- **GIVEN** a run still in progress with no summary yet, **WHEN** the feed renders, **THEN** the entry says it is still running rather than showing blank outcome text.

## Out of scope

- Re-running or retrying a run from the feed. Re-fire is the automation's own Run action.
- Retry-from-step, as n8n offers. Kandev runs are a single agent turn, not a node graph.
- A cross-workspace feed. `/automations` follows the active workspace.
- Removing the flat cross-automation feed. It is demoted from front door to lens, not deleted — with many automations "what happened overnight" is a real question a per-automation view cannot answer.
- A calendar of upcoming firings. The list's next-run cell is the first cut; a forward agenda is a later step.
- Pagination or infinite scroll. The capped limit is the first cut; revisit when a workspace outgrows it.
- Deleting runs from the feed. That stays in the per-automation log, where the delete-all confirmation already lives.
