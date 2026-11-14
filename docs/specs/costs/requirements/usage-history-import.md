---
status: draft
system: costs
created: 2026-11-01
owners:
  - kandev
---

# Usage History Import Requirements

## Overview

People who already use Kandev have months of agent work in local transcripts.
This capability lets them bring that usage into [Retained Usage History](usage-history.md) from the Session Cost plugin settings, and shows how the import is progressing.
It also makes sure one session that cannot be measured safely never stops the others from being measured.
The costs system owns this contract because it owns which sessions may contribute retained usage. The plugin supplies the source reads and settings surface.

## Terminology

- **Existing session:** A session whose row was created before retained usage capture was available.
- **Eligible session:** A session the Host authorizes for source collection.
- **Ineligible session:** A session the Host reports as unknown or mixed, so its transcript totals could include work outside its scope.
- **All-time step:** The first import step, which saves undated lifetime totals.
- **Daily step:** The second import step, which saves usage for each source day.
- **Active day:** A source day inside at least one eligible session's known date range.

## Requirements

### REQ-COSTS-USAGE-IMPORT-001: Existing session eligibility

**Intent:** Existing sessions can be imported without weakening source safety.

#### Acceptance criteria

- **AC-COSTS-USAGE-IMPORT-001.1:** When an upgrade completes, an existing session with a recorded transcript ID and no unsafe evidence shall become eligible without user action.
- **AC-COSTS-USAGE-IMPORT-001.2:** When a session is forked, its transcript ID is used in another workspace or organization, or its task has a recorded transfer, the session shall stay ineligible.
- **AC-COSTS-USAGE-IMPORT-001.3:** When a session has no recorded transcript ID, the session shall stay ineligible.
- **AC-COSTS-USAGE-IMPORT-001.4:** When an existing session becomes eligible, its actor attribution shall stay unknown unless execution or turn evidence proves it.
- **AC-COSTS-USAGE-IMPORT-001.5:** When eligibility adoption is interrupted, it shall resume without duplicate evidence, and it shall not walk all sessions again after it has completed.
- **AC-COSTS-USAGE-IMPORT-001.6:** When a workspace or organization is being erased or has been erased, adoption shall not create evidence for it.
- **AC-COSTS-USAGE-IMPORT-001.7:** When a task is archived, its sessions shall follow the same eligibility rules as sessions of an unarchived task.

### REQ-COSTS-USAGE-IMPORT-002: Two-step import

**Intent:** Users get all-time totals early, then daily charts without counting usage twice.

#### Acceptance criteria

- **AC-COSTS-USAGE-IMPORT-002.1:** When a user starts an import, the all-time step shall run one lifetime source report for the workspace before any daily source report runs.
- **AC-COSTS-USAGE-IMPORT-002.2:** When the all-time step completes, imported sessions shall contribute to all-time totals and stay outside dated charts until daily usage exists.
- **AC-COSTS-USAGE-IMPORT-002.3:** During the daily step, one run shall request each active day at most once, however many sessions share that day.
- **AC-COSTS-USAGE-IMPORT-002.4:** When both steps have saved usage for a session, its accepted usage shall contribute once to totals, and a partial daily result shall not reduce a fuller lifetime total.
- **AC-COSTS-USAGE-IMPORT-002.5:** When a completed import runs again over unchanged transcripts, retained totals shall not change and no observation revision shall increase.
- **AC-COSTS-USAGE-IMPORT-002.6:** When a source report contains rows for sessions outside the workspace's eligible sessions, those rows shall not be saved for that workspace.
- **AC-COSTS-USAGE-IMPORT-002.7:** When a source report finishes, the next report in the same import shall wait at least as long as the previous report took, with a minimum of 5 seconds and a maximum of 60 seconds.
- **AC-COSTS-USAGE-IMPORT-002.8:** When statistics collection is disabled, starting an import shall be refused with guidance to enable it, and a running import shall stop with a resumable disabled status.

### REQ-COSTS-USAGE-IMPORT-003: Import progress

**Intent:** Users can tell what the import is doing, how far it has got, and what it could not import.

#### Acceptance criteria

- **AC-COSTS-USAGE-IMPORT-003.1:** While an import is active, the settings card shall show the current step and, during the daily step, completed and total active days and the source day in progress.
- **AC-COSTS-USAGE-IMPORT-003.2:** When the all-time step has finished, the settings card shall show counts of sessions found, imported, without a local transcript, and skipped as ineligible.
- **AC-COSTS-USAGE-IMPORT-003.3:** When at least three active days have completed in the current run, the settings card shall show an estimated time remaining. Before that, it shall show no estimate.
- **AC-COSTS-USAGE-IMPORT-003.4:** When a user leaves the settings page and returns, the card shall show the current status of the selected workspace's import, and the import shall not depend on the page staying open.
- **AC-COSTS-USAGE-IMPORT-003.5:** Desktop and phone layouts shall expose the same status, counts, and actions. The progress bar shall expose its current and maximum values to assistive technology.
- **AC-COSTS-USAGE-IMPORT-003.6:** When an import is queued, the card shall say it is waiting for another import and shall not reveal which workspace is ahead of it.
- **AC-COSTS-USAGE-IMPORT-003.7:** When sessions cannot be imported, the completed status shall keep them as labelled counts and shall not describe the import as covering all history.

### REQ-COSTS-USAGE-IMPORT-004: Interruption and concurrency

**Intent:** A long import survives restarts and never interferes with another workspace.

#### Acceptance criteria

- **AC-COSTS-USAGE-IMPORT-004.1:** When the plugin restarts with an import running, queued, or preparing, and collection is enabled, the import shall resume from its last saved position without user action.
- **AC-COSTS-USAGE-IMPORT-004.2:** Only an explicit user cancellation shall produce the cancelled status. A plugin or Host shutdown shall not.
- **AC-COSTS-USAGE-IMPORT-004.3:** When a user starts an import while another workspace's import is active, the new import shall be queued in start order and the active import shall continue.
- **AC-COSTS-USAGE-IMPORT-004.4:** When a source report fails three times in a row for one day, the import shall stop with a failed status, keep its position, and offer Resume, which continues from that day.
- **AC-COSTS-USAGE-IMPORT-004.5:** When the import worker stops for any reason other than process exit, the persisted status shall not stay running or preparing.
- **AC-COSTS-USAGE-IMPORT-004.6:** When access to the workspace is lost or the workspace is erased, the import checkpoint shall be discarded, and its progress shall not be shown.

### REQ-COSTS-USAGE-IMPORT-005: Per-session isolation

**Intent:** One ineligible session affects only itself.

#### Acceptance criteria

- **AC-COSTS-USAGE-IMPORT-005.1:** When background collection includes an ineligible session, it shall skip that session and collect the eligible sessions in the same pass.
- **AC-COSTS-USAGE-IMPORT-005.2:** When a skipped session later becomes eligible while still active, a later collection pass shall collect it.
- **AC-COSTS-USAGE-IMPORT-005.3:** When an import includes an ineligible session, it shall count that session as skipped and continue, and it shall not ask the user to upgrade Kandev.
- **AC-COSTS-USAGE-IMPORT-005.4:** When a user opens cost details for an eligible existing session with collection disabled, the composer shall show live cost as it did before retained usage.
- **AC-COSTS-USAGE-IMPORT-005.5:** When a user opens cost details for an ineligible session, the composer shall state that its cost cannot be attributed safely, and it shall not ask the user to upgrade Kandev.
- **AC-COSTS-USAGE-IMPORT-005.6:** When a Host read fails transiently during collection, that pass shall stop without losing saved work, and the next pass shall retry.
- **AC-COSTS-USAGE-IMPORT-005.7:** When the Host's retained usage API is unavailable, the composer and settings card shall say that retained usage is unavailable and name both remedies: updating Kandev and turning on the usage history toggle. They shall not name only an upgrade.

## Out of scope

- Reading transcripts from remote SSH or container executors.
- Reconstructing usage for tasks or sessions deleted before the upgrade, or for transcripts that no longer exist.
- A single action that imports every workspace.
- Showing a session-count preview before the user starts an import.
- Assigning historical usage to a user without execution or turn evidence.

## Implementation Plans

- [Usage History Import package](../../../plans/usage-history-import/plan.md)
- [System design](../system-design/usage-history-import.md)
