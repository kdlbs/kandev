---
id: coordinator-copilot-activity
title: Coordinator copilot activity display
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# Coordinator copilot activity display Requirements

## Overview

The copilot panel shows that the coordinator is working without showing how
Kandev runs it. Terms are defined in the
[copilot requirements](copilot.md#terminology).

## Requirements

### REQ-COORDINATOR-COPILOT-006: Activity display

**Intent:** The manager sees that the coordinator is working, not how Kandev
runs it.

This requirement changes mockup `p1-05`, which shows each Kandev tool call as
its own row. It follows the pattern of assistant chats aimed at non-developers:
calm by default, detail on demand.

#### Acceptance criteria

- **AC-COORDINATOR-COPILOT-006.1:** While a turn is running, the panel shall
  show one status line above the composer that updates in place with a plain
  verb for the current tool (for example "Reading tasks", "Checking
  workflows", "Drafting a proposal") and the elapsed seconds. Tool calls shall
  not render as one row each while the turn runs.
- **AC-COORDINATOR-COPILOT-006.2:** When a turn ends, its tool calls shall
  collapse into one chip naming how many were made and how long the turn took,
  collapsed by default; expanding it shows one row per tool call with its
  existing detail.
- **AC-COORDINATOR-COPILOT-006.3:** A `propose_task_kandev` call shall never be
  collapsed: its proposal card always renders in full.
- **AC-COORDINATOR-COPILOT-006.4:** Once the agent has started successfully, the
  session start-up rows (environment preparation and agent start) shall be
  hidden; while the agent is still starting, or when it failed to start, they
  shall stay visible.
- **AC-COORDINATOR-COPILOT-006.5:** The activity display applies to the
  coordinator panel only; Settings configuration chat, Quick Chat and the
  task page shall render as before.
- **AC-COORDINATOR-COPILOT-006.6:** While a turn runs, none of its activity shall
  render as rows, except a tool call awaiting a permission decision (with
  Approve and Deny) and a `propose_task_kandev` card once its call returns.
- **AC-COORDINATOR-COPILOT-006.7:** There shall be one chip per turn, holding its
  tool calls even with agent text between them; its label shall state the call
  count, the duration (`Ns`, `Nm Ss`, `Nh Mm`) and, as text, any failed calls. A
  turn with no chippable call shall have no chip.
- **AC-COORDINATOR-COPILOT-006.8:** A stopped turn or a turn of a failed session
  shall end like any other: its calls collapse and the status line goes.
- **AC-COORDINATOR-COPILOT-006.9:** The status line shall show "Working" when no
  tool is running or the tool is unknown, count from the turn's first message
  (so it survives a reload), announce only verb changes, and not show while the
  session is starting.
