---
id: "02-archive-helper-history"
title: "Archive and reopen helper history"
status: done
wave: 2
depends_on:
  - "01-preserve-session-queue-work"
plan: "plan.md"
requirements:
  - REQ-TASKS-TERMINAL-SESSION-HISTORY-001
acceptance_criteria:
  - AC-TASKS-TERMINAL-SESSION-HISTORY-001.1
  - AC-TASKS-TERMINAL-SESSION-HISTORY-001.2
  - AC-TASKS-TERMINAL-SESSION-HISTORY-001.3
  - AC-TASKS-TERMINAL-SESSION-HISTORY-001.4
  - AC-TASKS-TERMINAL-SESSION-HISTORY-001.5
  - AC-TASKS-TERMINAL-SESSION-HISTORY-001.6
  - AC-TASKS-TERMINAL-SESSION-HISTORY-001.7
  - AC-TASKS-TERMINAL-SESSION-HISTORY-001.8
system_design:
  - ../../specs/tasks/system-design/terminal-session-history.md
---

# Task 02: Archive and reopen helper history

## Summary

Let the current primary clean up terminal helper sessions while preserving
useful transcripts and making archived conversations explicitly available
across desktop, kanban preview, and mobile.

## In scope

- Caller, task, workspace, target, state, and primary-session guards.
- Retain transcript evidence in an archived session with an archive timestamp
  and task-scoped cleanup receipt.
- Hard-delete disposable terminal session state when no retained evidence
  requires history.
- Exclude archived sessions from primary selection, automatic and manual
  recovery, and new prompt admission. The stopped-session banner hides
  same-session recovery actions for archived history.
- Localized history visibility and transcript reopening on all three surfaces.

## Out of scope

- Stopping or settling active sessions.
- Restoring an archived session to active state.
- Deleting a task workspace or repository.

## UI preview

See [UI-01 through UI-04](plan.md#ui-preview). This work order changes the
history toggle and archived-row visibility; desktop and preview use a compact
accessible control, while mobile uses the same localized action as a full-width
touch target.

## Acceptance

- Cleanup archives terminal helper sessions with retained conversation
  evidence and preserves transcript access; disposable sessions are removed.
- Archived sessions cannot become primary, resume, or receive new prompts, and
  retrying cleanup returns the original receipt.
- Users can hide or reveal archived history and reopen the preserved transcript
  on desktop, kanban preview, and mobile.

## Verification

Run focused repository and service tests for archive disposition, evidence
guards, receipt replay, and archived summary projection. Run the desktop history,
kanban preview history, and mobile session cleanup browser journeys; inspect the
captured desktop and phone views.

## Results

The accepted candidate `6aabf4bbf4e69fcadec352c3bf9ddaa080df6d62` passed
independent Review and QA. The inspected desktop archive/reopen capture was
executed on `4fe5501a4755b245e29f69c2b672274ca3cf8b05`; earlier inspected
desktop, preview, and phone captures were executed on
`e3a7eec3f14a2baad5b99433658c79c7b83417b1` and cover unchanged layouts. The
final service-summary fallback correction was verified separately on the
accepted candidate. A follow-up implementation guard rejects archived rows in
resume candidate admission and promptable-session loading; the resumption hook
also skips automatic inspection and manual resume for archived rows. Focused
backend and hook regression checks cover those paths. This local successor's
component test also verifies that the stopped-session banner keeps only the
task-scoped New Agent action for archived history. Browser evidence for the
banner correction remains pending revalidation; earlier history captures are
listed above and are not claimed as a browser run for this source successor.
