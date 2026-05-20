---
status: shipped
created: 2026-04-28
owner: cfl
---

# Subtasks as Workflow Checklist

## Why

When a CEO agent breaks a feature into an ordered sequence of subtasks — spec, plan,
build, review, PR — the parent task detail shows a flat list with no sense of progression.
Operators cannot tell at a glance which step is active, how far along the sequence is, or
whether the agent is stuck on an early step. The flat list also fails to communicate that
these tasks have a deliberate order enforced by blocker relationships.

## What

- When a parent task's children form one or more sequential chains (connected by `blocked_by`
  relationships), the subtasks section renders as a vertical stepper instead of a flat list.
- Each step shows: step number, status icon, identifier (monospace), and title.
- The active step — the first non-completed, non-cancelled step in the chain — is
  highlighted with the primary accent color and bold text.
- Completed steps show a filled checkmark icon and dimmed text.
- Future (pending/blocked) steps show an unfilled circle icon and muted text.
- Failed steps show a filled X icon and destructive accent.
- A continuous vertical connecting line runs between step nodes (stepper chrome).
- When children have **no** blocker relationships, the current flat list is rendered
  unchanged (fallback to existing `ChildIssuesList` behavior).
- Clicking any step navigates to that subtask's detail page.
- The stepper is read-only — no drag-to-reorder, no inline status changes.

### Ordering

Children are sorted by the workflow-sort algorithm: topological order following
`blocked_by` chains, with creation time as the tie-breaker within each ready group.
Topological sort using the `workflowSort` algorithm.
Parallel branches (a node with two successors, or a node with two blockers) fall back
to creation-time ordering within that group; the chain-walk stops at branch/merge points.

### Detection heuristic

Render as a stepper when at least one child has a non-empty `blockedBy` list pointing
to a sibling. If no child references a sibling blocker, render as a flat list.

## Scenarios

- **GIVEN** a parent task with 5 subtasks in a linear chain (spec → plan → build →
  review → PR, each blocked by the previous), **WHEN** viewing the parent detail,
  **THEN** the subtasks section renders as a 5-step vertical stepper with the first
  non-done step highlighted in the primary color.

- **GIVEN** subtask "spec" and "plan" are `done`, "build" is `in_progress`, **WHEN**
  viewing the checklist, **THEN** steps 1 and 2 show filled checkmarks with dimmed text,
  step 3 shows a highlighted active indicator, and steps 4–5 show unfilled circles with
  muted text.

- **GIVEN** subtask "build" transitions to `done` via a real-time WS update, **WHEN** the
  update arrives, **THEN** the stepper re-evaluates and highlights "review" as the new
  active step without a full page reload.

- **GIVEN** a parent task with 5 subtasks that have no blocker relationships, **WHEN**
  viewing the parent detail, **THEN** the subtasks render as the existing flat list
  (no stepper).

- **GIVEN** a parent task whose subtasks form two independent linear chains, **WHEN**
  viewing the parent detail, **THEN** both chains render in a single stepper using
  workflow-sort order, with each chain's active step highlighted.

- **GIVEN** a step is clicked, **WHEN** the click registers, **THEN** the browser
  navigates to `/office/tasks/:childId`.

## Out of scope

- Drag-to-reorder subtasks.
- Inline status editing from within the stepper.
- Collapsing or expanding the stepper.
- Showing a progress percentage or aggregate count badge on the stepper itself
  (that lives on the parent task header; see `StageProgressBar`).
- Creating or deleting blocker relationships from this view.
- A `position` field on `Task` — ordering derives entirely from `blocked_by` chains and
  creation time; no new DB column is needed.
