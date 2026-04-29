---
status: draft
created: 2026-04-29
owner: Carlos Florencio
---

# Improve Kandev

## Why

Users who hit a bug or have a feature idea today have no in-app way to report it,
and even when they do, the report sits as text someone else has to act on. Make
filing an improvement a one-click action that produces a real, actionable task
the user's own agent picks up immediately — turning every report into a contribution.

## What

- An icon-only button in the kanban top bar (next to the GitHub and Stats
  controls) opens a task-creation dialog that is pre-configured for the
  kandev codebase: repository locked to `https://github.com/kdlbs/kandev`,
  base branch `main`, workflow set to a hidden `improve-kandev` workflow,
  description seeded with a starter template. The button has a tooltip
  ("Improve kandev") explaining its purpose.
- The dialog reuses the existing task-create UI, including prompt enhancement,
  image paste, and file attachments.
- The dialog explains the flow up front: the agent will implement the change,
  the user will test it, then the agent opens a PR. Brief copy positions this
  as the user contributing to kandev's future.
- An "Include recent logs" toggle (default on) attaches a context bundle to the
  task: recent backend logs, frontend logs, and a metadata snapshot. The bundle
  lives in a temporary folder and is referenced by file path in the task
  description so the agent can read it on demand.
- Submitting the dialog creates the task in the user's active workspace, clones
  the kandev repo if needed, and starts the agent on the first step.
- The `improve-kandev` workflow has three manually-advanced steps:
  - **Improve** — agent implements the change with TDD; adds E2E tests when the
    change touches user-facing flows.
  - **Test** — agent runs `make install` then `make dev` (auto ports), reports
    the URLs so the user can verify the change in a second kandev instance.
  - **PR** — agent invokes the `pr` skill to commit, push, and open a pull
    request against `main` in `kdlbs/kandev`.
- The `improve-kandev` workflow is hidden from the workflow management page in
  workspace settings and from the workflow picker in the standard task-create
  dialog. It is reachable only through the Improve Kandev entry point.
- A pre-flight check surfaces `gh auth` status from `/api/v1/system/health` and
  prevents submission with a clear error when GitHub auth is missing.

## Scenarios

- **GIVEN** the user opens the Improve Kandev dialog with the logs checkbox on,
  **WHEN** they submit a title and description, **THEN** a task is created in
  their active workspace, the description references three files in a temp
  folder (`metadata.json`, `backend.log`, `frontend.log`), and the agent starts
  on the **Improve** step.

- **GIVEN** the agent reports the implementation is complete on the **Improve**
  step, **WHEN** the user moves the task to **Test**, **THEN** the agent
  auto-starts with the test step prompt, runs `make install` and `make dev`,
  and reports the assigned URLs back to the user.

- **GIVEN** the user has verified the change works, **WHEN** they move the task
  to **PR**, **THEN** the agent invokes the `pr` skill and opens a pull request
  against `main` in `kdlbs/kandev`.

- **GIVEN** the standard task-create dialog or the workspace workflows settings
  page is open, **WHEN** the page lists workflows, **THEN** `improve-kandev`
  does not appear.

- **GIVEN** the user has not configured `gh auth`, **WHEN** they open the
  Improve Kandev dialog, **THEN** the dialog shows a blocking error referencing
  the health-check result and disables the submit button.

## Out of scope

- Automatic transitions between workflow steps (user moves manually).
- Rate limiting, quotas, or one-task-at-a-time guards.
- Log redaction or sensitive-value scrubbing.
- Manual upstream-URL configuration. The user is expected to have `gh`
  authenticated; during the PR step, the agent automatically forks
  `kdlbs/kandev` to the user's account when they lack write access on the
  upstream repo, and pushes directly otherwise. Manual fork/remote setup
  remains an optional advanced workflow but is not part of this feature.
- A generic feedback inbox or report archive; this feature produces tasks,
  not stored reports.
- Cleanup of the temporary log bundle directory; left to OS/temp policy.
- Windows-specific considerations for `make install` / `make dev`.
