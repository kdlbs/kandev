---
id: "06-operator-ui"
title: "Operator UI: workspace settings coordinators tab"
status: done
wave: 6
depends_on:
  - "05-operator-api"
plan: "plan.md"
requirements:
  - REQ-TASKS-COORDINATOR-AUTHORITY-001
  - REQ-TASKS-COORDINATOR-AUTHORITY-002
acceptance_criteria:
  - AC-TASKS-COORDINATOR-AUTHORITY-001.1
  - AC-TASKS-COORDINATOR-AUTHORITY-001.3
  - AC-TASKS-COORDINATOR-AUTHORITY-002.2
system_design:
  - ../../specs/tasks/system-design/coordinator-task-authority.md
---

# Task 06 — Operator UI: Workspace Settings Coordinators Tab

## Owner

Frontend

## Predecessors

05 (Operator API)

## Description

Add a new workspace settings tab "Coordinators" with:

1. Grants table listing active grants (task, scope, capabilities, granted by/at)
2. Grant creation dialog (task picker, scope selector, capability checkboxes, note)
3. Revoke confirmation dialog
4. Recent audit log table

Hidden behind the runtime flag `features.coordinatorTaskAuthority`.

## Verification

- `coordinator-api.ts` + `.test.ts`
- Component tests for grants table, dialog, revoke
- Tab route test
- Flag-off hides the tab
- One Playwright e2e spec
- i18n: all 5 languages, no em dashes
- Mobile parity
