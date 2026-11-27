---
status: draft
system: office
created: 2026-07-31
owners:
  - nova28
---
# Automation Runs Requirements

## Overview

An automation that produces a report — a nightly drift sweep, a dependency audit — is read daily and edited almost never. Its output was reachable only through the automation's own edit form: Settings → workspace → Automations → open the automation → scroll past the whole configuration → expand a collapsed section → click a row. Six steps into an editing surface to perform a reading task.

## Requirements

### REQ-OFFICE-AUTOMATION-RUNS-001: Automation Runs

**Intent:** An automation that produces a report — a nightly drift sweep, a dependency audit — is read daily and edited almost never. Its output was reachable only through the automation's own edit form: Settings → workspace → Automations → open the automation → scroll past the whole configuration → expand a collapsed section → click a row. Six steps into an editing surface to perform a reading task.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-RUNS-001.1:** The sidebar carries an **Automations** section directly under New Task, listing the workspace's automations by name with the same indicator color semantics as the agenda, including the latest-outcome rules in .19 through .22. The list IS the navigation: picking one opens its history, not a settings form. The section header opens the full list, which adds the next firing and what each one last said.
- **AC-OFFICE-AUTOMATION-RUNS-001.2:** There is no separate "Runs" nav row, and the destination is named for the object it lists: `/automations`, matching the sidebar section and the settings page, with the same `IconBolt`. `/runs` still resolves so shared links keep working. Two entries pointing at the same place is what made the destination hard to place: the automation is the object, so the nav lists automations.
- **AC-OFFICE-AUTOMATION-RUNS-001.3:** The desktop section shall read definitions while its header or rows are visible. A folded section shall keep its count current without requesting health summaries. A collapsed rail or hidden section shall not initiate reads for that builtin section.
- **AC-OFFICE-AUTOMATION-RUNS-001.4:** **Up next** — every automation ordered by when it fires, soonest first. The time leads the row; the name is secondary, because the name is what the sidebar already gave you.
- **AC-OFFICE-AUTOMATION-RUNS-001.5:** The time is the resolved next firing (`~00:00 tomorrow · GMT+8`) or the reason there isn't one. The tilde is deliberate: the scheduler ticks on an interval, so the time is approximate and the UI SHALL NOT imply a precision it does not have.
- **AC-OFFICE-AUTOMATION-RUNS-001.6:** Reasons an enabled automation will not fire, each shown in place of a time: a run is still open and `max_concurrent_runs` is reached; no schedule is set; the trigger itself is switched off. An automation that will not fire stays in the agenda holding its reason — dropping it would make the agenda look complete when it is not.
- **AC-OFFICE-AUTOMATION-RUNS-001.7:** A standing constraint sits under the agenda, which is exactly what it qualifies: scheduled automations only fire while kandev is running.
- **AC-OFFICE-AUTOMATION-RUNS-001.8:** **Recent runs** — the cross-automation feed inline, newest first, each entry naming its automation. "Did anything go wrong overnight" is the other half of why someone opens this page; behind a link, they would have to ask for it twice. The filtered lens stays one click away for digging.
- **AC-OFFICE-AUTOMATION-RUNS-001.9:** While the Automations section is visible, a scheduled or manually triggered run shall update its sidebar row without a page reload. An open run replaces the health dot with an animated running indicator and exposes the localized Running state to assistive technology; after the last open run finishes, the row returns to the non-running indicator defined in .19 and .20. The section shall stop refreshing its health summaries when it is folded, the desktop rail is collapsed, or the rail is not rendered.

- **AC-OFFICE-AUTOMATION-RUNS-001.10:** While the builtin Automations section remains visible in a foreground client, external creation, rename, and deletion shall update its rows without navigation or reload. When no definition read is active, a refresh shall start within ten seconds of the change. An active read shall be allowed to settle before the next automatic refresh, which shall start within ten seconds of settlement. Updated rows shall appear after a successful read that includes the change. This bound assumes available transport and normal browser timer scheduling.
- **AC-OFFICE-AUTOMATION-RUNS-001.11:** Definition refreshes shall display only the selected workspace's automations. Switching workspaces shall immediately hide previous-workspace rows. An older response shall not replace a newer accepted result, including an empty result.
- **AC-OFFICE-AUTOMATION-RUNS-001.12:** Refreshes shall preserve saved visibility, order, and expansion. A hidden section shall remain hidden after external changes. Foreground return shall request current definitions without requiring a workspace switch. Phone navigation shall retain its existing drawer and current automation rows while open.
- **AC-OFFICE-AUTOMATION-RUNS-001.19:** An enabled, non-paused automation with no open runs shall show its newest run's status color in the desktop sidebar and agenda, consistent with that run in the history details: failure and retry-scheduling failure are red, success is green, cancelled and retry-scheduled are amber, and skipped and archived are muted. Newest means the first run in that automation's newest-first history, not the last successful run or a run selected in the details. A later success shall clear an earlier failure indication after the updated result is read.
- **AC-OFFICE-AUTOMATION-RUNS-001.20:** Current activity shall take priority over outcome color: any open run keeps the running indication, including when a newer firing was skipped; otherwise a paused automation keeps its muted indicator even if its latest run failed. Paused includes a disabled automation or disabled configured schedule. A never-run automation shall retain its existing idle or paused indication and shall not gain an outcome label or last-run age.
- **AC-OFFICE-AUTOMATION-RUNS-001.21:** Where a newest run exists, its non-success, non-running status shall be visible without hover: Failed, Retry could not be scheduled, Cancelled, Skipped, Archived, or Retry scheduled. In the desktop sidebar this word shall precede the existing age, for example `Failed, 4m ago`; successful rows shall retain age-only metadata. Outcome text shall remain visible on paused rows, even though their indicator is muted. Outcome and age shall describe the same run. The agenda shall preserve its next-firing information and show the non-success outcome separately, without inventing a last-run age column.
- **AC-OFFICE-AUTOMATION-RUNS-001.22:** The phone navigation drawer's automation rows shall expose the same outcome colors, running/paused precedence, and visible non-success status as desktop. Existing row-tap navigation, drawer dismissal, touch targets of at least 44 px, and drawer scrolling shall remain intact. Across all three surfaces, outcome and current running/paused state shall be available to assistive technology; non-success outcomes shall be conveyed by visible status text in addition to color; and localized long status labels shall not overlap the name or cause horizontal page overflow. Activity cues retain their existing composition: desktop paused and agenda running indicators keep their existing dot plus accessible state text.
- **AC-OFFICE-AUTOMATION-RUNS-001.23:** While the Automations page or an automation's history page is open, a run recorded by any trigger, including the GitHub poller, a schedule, or a webhook, shall appear without navigation or reload, even when no run was open before. The page shall request current runs within two seconds of the run being recorded. Only clients with access to the automation's workspace shall learn that a run was recorded. Status changes of an open run continue on the live-refresh cadence of .9.

#### Run output capture

- **AC-OFFICE-AUTOMATION-RUNS-001.13:** When the selected run's session uses CLI passthrough, its transcript shall explain that terminal output is not captured there. The explanation shall remain visible for running and finished runs, including after a direct load or reload.
- **AC-OFFICE-AUTOMATION-RUNS-001.14:** The explanation shall tell users to disable CLI passthrough in the automation's agent profile for recorded output in future sessions. It shall not promise recovery of past output or change the profile.
- **AC-OFFICE-AUTOMATION-RUNS-001.15:** Once the selected session is identified as passthrough, its transcript shall show no empty-turn warning. Earlier synthetic warnings shall disappear from that view. The shared exclusion follows [the empty-turn notice contract](../../ui/requirements/empty-turn-notice.md).
- **AC-OFFICE-AUTOMATION-RUNS-001.16:** Capture guidance shall preserve stored prompts, agent messages, error entries, the reply composer, and the run's reported status. Viewing guidance shall not start, resume, resize, or send input to an agent.
- **AC-OFFICE-AUTOMATION-RUNS-001.17:** Desktop and phone users shall receive the same localized guidance within the selected run's content. Reading it shall require no hover, extra drawer, or horizontal page scrolling.
- **AC-OFFICE-AUTOMATION-RUNS-001.18:** When selection changes, capture guidance shall follow the selected session. A structured session shall retain its recorded transcript and normal empty-turn feedback. Later profile edits shall not reclassify an existing session's output mode.

## Exclusions

Definition freshness does not change automation configuration, scheduling, run
execution, or saved navigation preferences. It does not require a new event
protocol or shared cache.

Run output guidance does not capture, replay, or reconstruct terminal output.
Folder-trust prompts and early process-exit settlement have separate repair scopes.
Latest-outcome presentation does not change run selection, status derivation,
scheduling, retries, retention, transport, or saved navigation preferences.
Custom pinned-shortcut activity semantics are outside this capability.

## System design

- [Automation runs](../system-design/automation-runs.md) owns the existing history and refresh contracts.
- [Run output capture guidance](../system-design/automation-run-output.md) owns the passthrough output guidance.
- [Latest-run indicator](../system-design/automation-latest-run-indicator.md) owns the presentation extension for issue [#5081](https://github.com/kdlbs/kandev/issues/5081).
- [Run-recorded events](../system-design/automation-run-recorded-events.md) owns the push refresh in AC .23.
