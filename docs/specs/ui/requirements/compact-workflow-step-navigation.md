---
status: draft
system: ui
created: 2026-08-26
owners:
  - kandev
---

# Compact Workflow Step Navigation Requirements

## Overview

The task top bar replaces the full workflow stepper when its center area is too narrow. The compact form currently hides all other steps.

The UI system owns this responsive disclosure. The task system continues to own workflow order, move eligibility, and task transitions.

## Terminology

- **Compact stepper:** The current step name, state marker, and position count that replace the full stepper.
- **Step disclosure:** The temporary surface that shows all steps from the compact stepper.
- **Eligible step:** A workflow step that the existing task-move policy permits as a manual target.

## Requirements

### REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001: Compact workflow step navigation

**Intent:** Users need to inspect and select workflow steps when the task top bar cannot show the full stepper.

**User story:** As a task user, I want the compact stepper to show all steps, so that I can move the task without leaving it.

#### Acceptance criteria

- **AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.1:** When the full stepper cannot fit, the system shall show the current step name and its position in the workflow.
- **AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.2:** When a fine-pointer user hovers or focuses the compact stepper, the system shall show every workflow step in workflow order in an interactive dialog surface. The surface shall expose its move controls to keyboard users.
- **AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.3:** When a coarse-pointer user activates the compact stepper, the system shall show the same step choices in a touch surface. The trigger shall provide a minimum 44px hit area and a visible disclosure cue.
- **AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.4:** When the disclosure is open, the system shall identify the current step and enable each eligible target.
- **AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.5:** When a user selects an eligible target, the system shall use the existing task-move behavior and eligibility rules.
- **AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.6:** When the disclosure opens, it shall remain inside the viewport and provide keyboard navigation, including Tab to an eligible step-specific move control and Enter activation, Escape dismissal, and focus return. The trigger and dialog surface shall expose matching ARIA semantics.
- **AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.7:** When a task is archived, the compact presentation shall remain a static archived indicator and shall not offer task movement.
- **AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.8:** When the task top bar has sufficient space, the existing full-stepper presentation and move behavior shall remain unchanged.
- **AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.9:** When a phone does not show the task top bar, the mobile task drawer shall retain its existing **Move to** path.
- **AC-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001.10:** Fine-pointer movement controls shall retain compact desktop sizing. The 44px minimum shall apply only to coarse-pointer touch hit areas and mobile controls.

## Out of scope

- Changes to workflow order, task-move permissions, or backend task transitions.
- A new workflow control in the phone task top bar.
- Changes to the expanded stepper layout.
- Cross-workflow task movement from the compact stepper.
- New task-move notifications or error messages.
