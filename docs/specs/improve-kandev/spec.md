---
status: building
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

- The **Improve Kandev** action in the desktop app-sidebar footer opens a
  task-creation dialog that is pre-configured for the kandev codebase:
  repository locked to `https://github.com/kdlbs/kandev`, base branch `main`,
  workflow selected from the hidden Improve Kandev workflows, and description
  seeded with a starter template. On phones the same action is a 44px-or-larger
  row in the existing mobile home menu's **Utilities** section.
- The dialog reuses the existing task-create UI, including prompt enhancement,
  image paste, and file attachments.
- The dialog explains the flow up front: the agent will implement the change,
  the user will test it, then the agent opens a PR. Brief copy positions this
  as the user contributing to kandev's future.
- The explanation includes a "Do not show this again" preference. Once selected,
  later uses of **Improve Kandev** skip the explanation and open the
  pre-configured task-creation dialog directly. The preference is local to the
  current browser profile and can be cleared with other local UI state.
- The task-creation dialog offers three report kinds: **Bug fix**, **Feature
  request**, and **Open issue**. Bug fixes and feature requests use the existing
  implementation workflow. Open issue uses a separate hidden, one-step workflow
  and visibly explains that the agent only publishes a GitHub issue; it does not
  implement the change or open a pull request.
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
- Hidden workflows do not count as choices in the standard task-create dialog.
  When the active workspace has exactly one visible workflow, the dialog uses
  that workflow implicitly and omits the redundant workflow selector. This
  remains true when the standard dialog is opened from a task-detail route
  whose task belongs to a hidden workflow; only a feature wrapper that
  explicitly locks the workflow may create another task in that hidden
  workflow.
- The `report-kandev-issue` workflow is also hidden and reachable only through
  the **Open issue** option. Its agent reads the repository's current bug-report
  or feature-request issue form, gathers every required field from the user,
  checks for sensitive data and likely duplicates, then publishes the issue to
  `kdlbs/kandev` with the matching template and reports the issue URL. The agent
  must ask follow-up questions instead of inventing missing required details.
- A pre-flight check surfaces `gh auth` status from `/api/v1/system/health` and
  prevents submission with a clear error when GitHub auth is missing.
- An account that cannot fork `kdlbs/kandev` is blocked from the implementation
  workflows but may still use the issue-only workflow.

## API surface

- `POST /api/v1/system/improve-kandev/bootstrap` continues to accept
  `{ "workspace_id": string }`. Its success response includes the existing
  repository, branch, context-bundle, GitHub-login, write-access, and
  fork-status fields plus:
  - `workflow_id: string` — the workspace instance of `improve-kandev`.
  - `issue_workflow_id: string` — the workspace instance of
    `report-kandev-issue`.
- Both workflow IDs refer to hidden, workspace-scoped workflow instances and
  are safe to request repeatedly.

## Persistence guarantees

- The intro dismissal is stored as
  `kandev.improveKandev.skipIntro = "true"` in browser local storage. It
  survives reloads and Kandev restarts for that browser profile, but is not
  synchronized between browsers or users.
- The two hidden workflow instances are workspace-scoped and remain idempotent:
  opening the dialog again reuses the existing workflow for each template.

## Failure modes

- If browser local storage is unavailable, the preference read/write fails
  open and the intro remains available; opening Improve Kandev still works.
- If the saved preference skips the intro but GitHub authentication is missing,
  the GitHub-auth recovery explanation takes precedence over the direct-open
  preference.
- If bootstrap cannot create or resolve either hidden workflow, the task form
  remains blocked and surfaces the bootstrap error.
- A fork restriction blocks only **Bug fix** and **Feature request** submission.
  Switching to **Open issue** clears that restriction because publishing an
  issue requires neither a fork nor push access.

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
  page is open, **WHEN** the page lists workflows, **THEN** neither
  `improve-kandev` nor `report-kandev-issue` appears.

- **GIVEN** the intro explanation is visible, **WHEN** the user selects "Do not
  show this again" and later reopens **Improve Kandev**, **THEN** the
  task-creation dialog opens directly and the intro explanation is skipped.

- **GIVEN** the user opens the mobile home menu, **WHEN** they tap **Improve
  Kandev**, **THEN** the menu closes and the same intro or direct task-creation
  flow opens without horizontal document overflow.

- **GIVEN** the user selects **Open issue**, **WHEN** they create the task,
  **THEN** the task starts in the issue-only workflow and the agent gathers all
  required fields from the matching repository issue form before publishing a
  GitHub issue.

- **GIVEN** GitHub reports that the user cannot fork `kdlbs/kandev`, **WHEN**
  they select **Open issue**, **THEN** they can create the report task because
  that workflow does not require a fork.

- **GIVEN** the active workspace has one visible workflow and one or more hidden
  workflows, **WHEN** the user opens the standard task-create dialog without an
  explicit workflow, **THEN** the visible workflow is selected implicitly and
  the workflow selector does not appear.

- **GIVEN** the user is viewing a task that belongs to a hidden Improve Kandev
  workflow, **WHEN** they open the standard New Task dialog from either the
  desktop sidebar or the mobile task drawer and create a task, **THEN** the new
  task uses the workspace's visible workflow rather than inheriting the hidden
  task-detail workflow.

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
