---
status: active
system: tasks
created: 2026-10-28
owners:
  - kandev
---

# Walkthrough Generation Time Requirements

## Overview

People need to see when the current in-app code walkthrough was generated.
Tasks owns this contract because the timestamp identifies the saved walkthrough revision.
The user confirmed that this request applies to the in-app walkthrough.

## Terminology

Generation time means the time Kandev saved the current agent-authored walkthrough.
It does not mean generation started, the walkthrough was opened, or the task was created.

## Requirements

### REQ-TASKS-WALKTHROUGH-TIME-001: Visible generation time

**Intent:** Help readers identify the age of the walkthrough they are reading.

#### Acceptance criteria

- **AC-TASKS-WALKTHROUGH-TIME-001.1:** An open walkthrough shall show a labelled date and time to the minute for its current saved generation. The timezone shall appear only in the hover text.
- **AC-TASKS-WALKTHROUGH-TIME-001.2:** Replacing a walkthrough shall update the displayed generation time. Navigation, feedback, reopening, and page reload shall preserve that time.
- **AC-TASKS-WALKTHROUGH-TIME-001.3:** The label and date format shall follow the selected application language. The time shall use the viewer's local timezone.
- **AC-TASKS-WALKTHROUGH-TIME-001.4:** Desktop and phone walkthroughs shall expose the date and time without hover, left-aligned with the text inside the badge above. Text shall wrap without covering controls or causing horizontal overflow.
- **AC-TASKS-WALKTHROUGH-TIME-001.5:** If the generation time is missing or invalid, the walkthrough shall remain usable and omit the timestamp. It shall not invent a time.

## Out of scope

- Generated PR webpages and chat tool-result summaries.
- Generation duration, relative age timers, and stale-code warnings.
- New persistence fields, API contracts, or changes to walkthrough navigation.

## Implementation plans

- [Walkthrough generation time](../../../plans/walkthrough-generation-time/plan.md)
