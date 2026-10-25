---
status: active
system: agents
created: 2026-09-13
owners:
  - kandev
---

# Agent Goal Visibility Requirements

## Overview

An agent can retain an active goal and continue automatically after a turn ends.
Users need to distinguish that state from an agent waiting for a new instruction.
The agent system owns the provider goal contract and its visible chat outcomes.

## Requirements

### REQ-AGENTS-GOAL-VISIBILITY-001: Active goal disclosure

**Intent:** Show when the selected session has an active goal that can cause automatic continuation.

#### Acceptance criteria

- **AC-AGENTS-GOAL-VISIBILITY-001.1:** When the provider reports an active goal,
  the selected session shall show a static **Goal Active** chip above the chat input,
  after the Todos and PR information. The chip shall appear even without Todos or a PR.
- **AC-AGENTS-GOAL-VISIBILITY-001.2:** The chip shall remain visible between turns
  while the goal remains active. An idle thread, final response, or unrelated
  metadata update shall not clear the goal or imply completion.
- **AC-AGENTS-GOAL-VISIBILITY-001.3:** Hover, keyboard focus, and click shall expose
  the goal objective and status. The details shall explain that the agent may
  continue automatically between replies. They shall not promise a wakeup time.
- **AC-AGENTS-GOAL-VISIBILITY-001.4:** On phone or coarse-pointer devices, tapping
  the chip shall open the same details in a bottom drawer. Long objectives shall
  scroll within the drawer without document overflow. The trigger shall have a
  touch target of at least 44 CSS pixels, and dismissal shall restore focus when possible.
- **AC-AGENTS-GOAL-VISIBILITY-001.5:** Completion, clearing, pausing, blocking, or a
  usage/budget limit shall remove the active chip. Without an explicit valid active
  goal, the UI shall not infer one from prose, tools, task state, or recurring activity.
- **AC-AGENTS-GOAL-VISIBILITY-001.6:** Reopening, reloading, and reconnecting shall
  restore the latest accepted goal state for the same session. Late data shall
  not resurrect a cleared goal, regress completion, or expose another session's goal.
- **AC-AGENTS-GOAL-VISIBILITY-001.7:** Ordinary task chat and Quick Chat shall use
  the same goal state and disclosure behavior. Switching sessions shall close the
  previous details. Unsupported providers shall retain their existing UI.
- **AC-AGENTS-GOAL-VISIBILITY-001.8:** The chip and details shall be read-only.
  Opening or dismissing them shall not start a turn, resume an agent, change a goal,
  or send a provider request. Labels shall support the application's locales.

## Out of scope

- Goal creation, pause/resume/clear controls, provider scheduling, and polling cadence changes.
- Goal history, progress percentages, invented next-wakeup times, and live usage counters.
- Office/autopilot semantics, task lifecycle changes, and passthrough-terminal toolbar redesign.

## System design

- [Goal visibility](../system-design/goal-visibility.md)
