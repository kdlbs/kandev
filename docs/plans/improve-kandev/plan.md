---
spec: docs/specs/workspaces/requirements/improve-kandev.md
created: 2026-07-27
status: done
---

# Implementation Plan: Improve Kandev Dialog and Issue Reporting

## Overview

Extend the existing Improve Kandev bootstrap so it provisions both the
implementation workflow and a one-step issue-reporting workflow, then let the
shared task-create surface select the correct hidden workflow. Add a
browser-local intro dismissal, preserve GitHub-auth and fork safeguards, and
expose the same flow from the existing mobile home menu. The work is partially
implemented in the current uncommitted worktree; tasks below identify exactly
what is complete and what must be continued.

Relevant workflow decision:
[Single-Session Model Switching](../../decisions/2026-07-26-single-session-model-switching.md).

## Planning Guardrail

The session learning that triggered this plan is already encoded in:

- `AGENTS.md`
- `.agents/skills/spec-driven-development/SKILL.md`
- `apps/backend/config/workflows/improve-kandev.yml`

These files now state that workflow-generated implementation envelopes do not
skip spec/plan/task creation. See
[Task 01](task-01-harness-planning-gate.md).

---

## Backend

### Hidden issue-reporting workflow

- Add `apps/backend/config/workflows/report-kandev-issue.yml` with template ID
  `report-kandev-issue`.
- Keep it hidden and give it one auto-starting **Open issue** step.
- The prompt must read the current repository issue forms, classify bug versus
  feature, gather every required field through `ask_user_question_kandev`,
  reject public security disclosure, check duplicates, preview the complete
  public draft, obtain publication approval, and run `gh issue create`.
- Extend `apps/backend/config/workflows/loader_test.go` to prove the template is
  present, hidden, single-step, auto-starting, and contains the required
  reporting safeguards.

### Improve Kandev bootstrap

- Extend `BootstrapResponse` in
  `apps/backend/internal/improvekandev/handler.go` with
  `IssueWorkflowID string 'json:"issue_workflow_id"'`.
- Generalize `ensureWorkflow` so bootstrap idempotently resolves or creates
  both `improve-kandev` and `report-kandev-issue`, healing the hidden flag for
  either stale workspace instance.
- Return both workflow IDs in the same bootstrap response; preserve repository
  reuse/cloning, bundle generation, GitHub login, write access, and fork-status
  behavior.
- Add a handler integration test in
  `apps/backend/internal/improvekandev/handler_test.go` using a real test
  SQLite task/workflow repository plus fake cloner and GitHub probe. POST the
  bootstrap route twice and assert stable, distinct workflow IDs, hidden
  workspace workflows, and the issue workflow's start step.

---

## Frontend

### API and dialog model

- Add `issue_workflow_id` to
  `ImproveKandevBootstrapResponse` in
  `apps/web/lib/api/domains/improve-kandev-api.ts`.
- Load steps for both workflow IDs during bootstrap.
- Extract the non-rendering decisions currently accumulating in
  `improve-kandev-dialog.tsx` and
  `improve-kandev-dialog-create.tsx` into a focused pure model/helper with
  Vitest coverage:
  - read/write `kandev.improveKandev.skipIntro` safely when local storage is
    unavailable;
  - choose intro versus create mode;
  - choose implementation versus issue workflow and start step;
  - block implementation kinds, but not **Open issue**, for `blocked_emu`.
- Keep GitHub-auth recovery authoritative: a saved intro dismissal must not
  bypass a missing-auth message.

### Intro and task kind selection

- Add a controlled **Do not show this again** checkbox to the existing intro.
  The labeled touch target is at least 44px high.
- Add **Open issue** beside **Bug fix** and **Feature request**.
- When selected, switch the locked workflow/default step to
  `issue_workflow_id`, use issue-specific placeholder copy, default log capture
  off, show an explicit report-only notice, and show only the one-step workflow
  preview.
- The notice states that the agent follows the repository issue template, asks
  for missing information and approval, creates an issue, and does not
  implement code or open a pull request.
- Retain the existing contributor banner for implementation kinds. For issue
  reporting, explain that a fork and push access are not required.

### Mobile design contract

- **Desktop outcome:** the app-sidebar footer action opens the intro or direct
  create dialog.
- **Mobile entry point:** add a 44px-or-larger **Improve Kandev** row to the
  existing `MobileMenuSheet` Utilities section.
- **Nearest exemplar:** `apps/web/components/kanban/mobile-menu-sheet.tsx`;
  reuse its inset full-height drawer, fixed header, internal scrolling body,
  safe-area padding, and utility-row geometry.
- **Hierarchy and primary action:** tapping the utility row closes the menu,
  then opens the shared Improve Kandev dialog. The task-create dialog remains
  the single focal surface; no stacked drawer/dialog.
- **Scroll and state:** the mobile menu remains the drawer's single scroll
  owner, the intro dialog is viewport-contained with internal vertical
  scrolling, and the dialog state/business logic is shared across viewports.
- Refactor `MobileMenuSheet` and `CreateModeView` so touched functions remain
  within the repository's line, complexity, and no-nested-ternary limits.

---

## Tests

- **What:** embedded issue workflow contract.
  **File:** `apps/backend/config/workflows/loader_test.go`.
  **How:** load embedded templates and assert ID, hidden flag, one step,
  auto-start event, and prompt safeguards.
- **What:** bootstrap creates/reuses both hidden workflow instances.
  **File:** `apps/backend/internal/improvekandev/handler_test.go`.
  **How:** Gin handler integration test with real SQLite repositories and fake
  external boundaries; call twice and inspect response plus persisted workflow
  state.
- **What:** local-storage preference, workflow selection, and fork-block
  decisions.
  **File:** a focused `*.test.ts` beside the extracted dialog model/helper.
  **How:** table-driven Vitest cases using an in-memory Storage-compatible fake.
- **What:** description log bundle remains compatible with the extended
  bootstrap response.
  **File:** `apps/web/components/improve-kandev-dialog-helpers.test.ts`.
  **How:** existing Vitest cases with both workflow IDs in the fixture.

---

## E2E Tests

- **Scenario:** selecting the intro dismissal persists through reload and later
  opens task creation directly.
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`.
  **Verify:** local-storage value, reload, direct create dialog, and absent
  intro copy.
- **Scenario:** selecting **Open issue** creates a task in the issue workflow.
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`.
  **Verify:** report-only notice, submitted task title, and issue workflow start
  step.
- **Scenario:** an EMU-shaped account cannot submit implementation work but can
  submit an issue report.
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`.
  **Verify:** disabled submit for Bug fix/Feature request and enabled submit
  after selecting Open issue.
- **Scenario:** a phone user opens Improve Kandev from the home menu and reaches
  the issue-only option.
  **File:** `apps/web/e2e/tests/mobile-improve-kandev.spec.ts`.
  **Verify:** visible 44px utility/preference targets, menu dismissal, issue
  notice, and no document horizontal overflow.

---

## Completion Snapshot

The implementation, focused tests, desktop/mobile E2E coverage, repository
verification, review fixes, and merge-conflict resolution are complete. The PR
is the authoritative record for the final commit and CI results.

---

## Implementation Waves And Parallel Candidates

The default is sequential execution in this primary conversation.

Planning:

- [x] [Task 01: Harness planning gate](task-01-harness-planning-gate.md)

Wave 1:

- [x] [Task 02: Backend issue workflow and bootstrap](task-02-backend-issue-workflow.md)
  — complete; loader, bootstrap wiring, and real SQLite integration coverage pass.

Wave 2:

- [x] [Task 03: Dialog persistence, workflow selection, and mobile entry](task-03-frontend-dialog-and-mobile.md)
  — complete; model tests, responsive entry, and lint/typecheck pass.

Wave 3:

- [x] [Task 04: Desktop and mobile E2E coverage](task-04-e2e-coverage.md)
  — complete; rebuilt desktop and mobile Playwright suites pass.

Wave 4:

- [x] [Task 05: Required verification and commit](task-05-verification-and-commit.md)
  — verification complete; changes are ready for the final commit.

No tasks are marked parallel-safe: Tasks 02–04 share the bootstrap contract and
Improve Kandev test fixtures, and Task 05 depends on all of them.
