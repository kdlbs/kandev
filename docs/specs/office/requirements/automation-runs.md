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

- **AC-OFFICE-AUTOMATION-RUNS-001.1:** The sidebar carries an **Automations** section directly under New Task, listing the workspace's automations by name with the same health dot the list uses. The list IS the navigation: picking one opens its history, not a settings form. The section header opens the full list, which adds the next firing and what each one last said — more than a sidebar row should try to hold.
- **AC-OFFICE-AUTOMATION-RUNS-001.2:** There is no separate "Runs" nav row, and the destination is named for the object it lists: `/automations`, matching the sidebar section and the settings page, with the same `IconBolt`. `/runs` still resolves so shared links keep working. Two entries pointing at the same place is what made the destination hard to place: the automation is the object, so the nav lists automations.
- **AC-OFFICE-AUTOMATION-RUNS-001.3:** The section fetches only while it is on screen — a collapsed rail or a folded section issues no requests, since the sidebar is mounted on every page.
- **AC-OFFICE-AUTOMATION-RUNS-001.4:** **Up next** — every automation ordered by when it fires, soonest first. The time leads the row; the name is secondary, because the name is what the sidebar already gave you.
- **AC-OFFICE-AUTOMATION-RUNS-001.5:** The time is the resolved next firing (`~00:00 tomorrow · GMT+8`) or the reason there isn't one. The tilde is deliberate: the scheduler ticks on an interval, so the time is approximate and the UI SHALL NOT imply a precision it does not have.
- **AC-OFFICE-AUTOMATION-RUNS-001.6:** Reasons an enabled automation will not fire, each shown in place of a time: a run is still open and `max_concurrent_runs` is reached; no schedule is set; the trigger itself is switched off. An automation that will not fire stays in the agenda holding its reason — dropping it would make the agenda look complete when it is not.
- **AC-OFFICE-AUTOMATION-RUNS-001.7:** A standing constraint sits under the agenda, which is exactly what it qualifies: scheduled automations only fire while kandev is running.
- **AC-OFFICE-AUTOMATION-RUNS-001.8:** **Recent runs** — the cross-automation feed inline, newest first, each entry naming its automation. "Did anything go wrong overnight" is the other half of why someone opens this page; behind a link, they would have to ask for it twice. The filtered lens stays one click away for digging.
- **AC-OFFICE-AUTOMATION-RUNS-001.9:** While the Automations section is visible, a scheduled or manually triggered run shall update its sidebar row without a page reload. An open run replaces the health dot with an animated running indicator and exposes the localized Running state to assistive technology; after the last open run finishes, the row returns to its non-running health indicator. The section shall stop refreshing its health summaries when it is folded, the desktop rail is collapsed, or the rail is not rendered.

## System design

The migrated technical source is split into [part 1](../system-design/automation-runs.md).
