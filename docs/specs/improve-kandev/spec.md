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
- Submitting the dialog creates the task in the dedicated **Improve Kandev**
  workspace, clones the kandev repo if needed, and starts the agent on the
  first step.
- The dedicated workspace is created automatically on first bootstrap and
  reused on every later use, keeping improve tasks isolated and segregated
  from the user's regular work. It is named `Improve Kandev`, is a normal
  visible workspace (with a kanban workflow), and persists across restarts.
- The `improve-kandev` workflow has three manually-advanced steps:
  - **Improve** — agent implements the change with TDD; adds E2E tests when the
    change touches user-facing flows.
  - **Test** — agent runs `make install` then `make dev` (auto ports), reports
    the URLs so the user can verify the change in a second kandev instance.
  - **PR** — agent invokes the `pr` skill to commit, push, and open a pull
    request against `main` in `kdlbs/kandev`.
- The `improve-kandev` workflow is hidden from the workflow management page in
  workspace settings and from the workflow picker in the standard task-create
  dialog, except in the dedicated `Improve Kandev` workspace itself where the
  workflows settings page lists it (and `report-kandev-issue`) **read-only**.
  It is reachable through the Improve Kandev entry point.
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

- `POST /api/v1/system/improve-kandev/bootstrap` accepts an optional
  `{ "workspace_id": string, "create_workspace": boolean }`. When the
  dedicated `Improve Kandev` workspace exists, both fields are ignored and
  everything is scoped to it. When it is missing, `create_workspace: true`
  creates it (with the GitHub connection carried over from the user's default
  workspace); `create_workspace: false` falls back to `workspace_id` (the
  user's active workspace, legacy behavior). Its success response includes
  the existing repository, branch, context-bundle, GitHub-login, write-access,
  and fork-status fields plus:
  - `workspace_id: string` — the dedicated Improve Kandev workspace the task
    must be created in.
  - `workflow_id: string` — the workspace instance of `improve-kandev`.
  - `issue_workflow_id: string` — the workspace instance of
    `report-kandev-issue`.
- Both workflow IDs refer to hidden, workspace-scoped workflow instances in
  the returned workspace and are safe to request repeatedly.

## Persistence guarantees

- The intro dismissal is stored as
  `kandev.improveKandev.skipIntro = "true"` in browser local storage. It
  survives reloads and Kandev restarts for that browser profile, but is not
  synchronized between browsers or users.
- The dedicated `Improve Kandev` workspace is a normal persisted workspace
  row created on first bootstrap and reused thereafter; it survives restarts.
- The two hidden workflow instances live in the dedicated workspace and remain
  idempotent: opening the dialog again reuses the existing workflow for each
  template.

## Failure modes

- If browser local storage is unavailable, the preference read/write fails
  open and the intro remains available; opening Improve Kandev still works.
- If the saved preference skips the intro but GitHub authentication is missing,
  the GitHub-auth recovery explanation takes precedence over the direct-open
  preference.
- If the dedicated workspace cannot be created or resolved, bootstrap fails
  and the dialog surfaces the error with the task form blocked.
- Concurrent bootstrap calls that race the workspace creation converge on a
  single workspace: a creation failure re-reads the workspace list and reuses
  an existing `Improve Kandev` row.
- If bootstrap cannot create or resolve either hidden workflow, the task form
  remains blocked and surfaces the bootstrap error.
- A fork restriction blocks only **Bug fix** and **Feature request** submission.
  Switching to **Open issue** clears that restriction because publishing an
  issue requires neither a fork nor push access.

## Scenarios

- **GIVEN** the user opens the Improve Kandev dialog with the logs checkbox on,
  **WHEN** they submit a title and description, **THEN** a task is created in
  the dedicated `Improve Kandev` workspace (created automatically on first
  use), the description references three files in a temp folder
  (`metadata.json`, `backend.log`, `frontend.log`), and the agent starts on
  the **Improve** step.

- **GIVEN** no `Improve Kandev` workspace exists, **WHEN** bootstrap is called,
  **THEN** a workspace named `Improve Kandev` is created, the kandev
  repository and both hidden workflows live in it, and the response includes
  its `workspace_id`.

- **GIVEN** an `Improve Kandev` workspace already exists, **WHEN** bootstrap is
  called again, **THEN** the same workspace (and the same hidden workflow
  instances) are reused and the response's `workspace_id` is unchanged.

- **GIVEN** the user's active workspace is not the dedicated workspace,
  **WHEN** the dialog submits a task, **THEN** the task appears in the
  dedicated workspace and no task is created in the active workspace.

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

- **GIVEN** the user opens the dedicated workspace's workflows settings page,
  **WHEN** the page loads, **THEN** it lists `improve-kandev` with steps
  Improve → Test → PR and `report-kandev-issue` read-only, and no
  create/edit/delete/import/export controls are available.

- **GIVEN** the user opens the dedicated workspace's repositories settings
  page, **WHEN** the page loads, **THEN** the registered kandev repository is
  listed read-only and no add/edit/delete controls are available.

- **GIVEN** a mutation request (workflow create/update/delete/reorder/step
  mutation or repository create/update/delete/initialize) is scoped to the
  dedicated workspace, **WHEN** it reaches the backend, **THEN** it is
  rejected with HTTP 409 before any write.

- **GIVEN** bootstrap creates the `Improve Kandev` workspace for the first
  time and the user's default workspace has a GitHub connection, **WHEN** the
  workspace is created, **THEN** the new workspace carries the same GitHub
  connection (and PAT secret where applicable), and no other integration
  configurations, automations, workflows, or repositories beyond the bootstrap
  defaults.

- **GIVEN** bootstrap creates the `Improve Kandev` workspace for the first
  time and the user's default workspace has **no** GitHub connection, **WHEN**
  the workspace is created, **THEN** no connection is copied and the workspace
  starts without a GitHub connection.

- **GIVEN** the `Improve Kandev` workspace already exists, **WHEN** bootstrap
  is called again, **THEN** the workspace's configuration (including its
  GitHub connection) is untouched — nothing is re-copied.

- **GIVEN** the dedicated workspace does not exist, **WHEN** the user opens
  the Improve Kandev dialog, **THEN** the dialog offers a "Create a dedicated
  Improve Kandev workspace" checkbox (default checked) — in the intro, or as a
  gate before the create form when the intro was dismissed.

- **GIVEN** the dedicated workspace does not exist and the user unchecks the
  creation checkbox, **WHEN** they proceed, **THEN** bootstrap falls back to
  the active workspace: the hidden workflows and kandev repo are scoped there
  and the task lands in it (legacy behavior).

## Dedicated workspace immutability

- The dedicated `Improve Kandev` workspace is configuration-immutable for the
  surfaces that define how tasks run there:
  - **Workflows**: the settings page
    (`/settings/workspace/<id>/workflows`) lists the workspace's workflows —
    including the hidden `improve-kandev` (Improve → Test → PR) and
    `report-kandev-issue` instances — read-only. Creating, editing, deleting,
    reordering, importing, exporting, and GitHub-syncing workflows is not
    possible; no workflow may be added. Step mutations are equally rejected.
  - **Repositories**: the settings page lists the workspace's repositories
    (the registered kandev repo) read-only. Creating, initializing, editing,
    and deleting repositories is not possible.
- Enforcement is backend-side: the workflow and repository mutation endpoints
  reject requests scoped to the dedicated workspace (HTTP 409) before any
  write. The settings pages additionally hide/disable the mutation controls so
  the state is visible up front.
- Task creation, agent runs, step advancement, and the kanban board are
  unaffected — only configuration mutations are restricted.

## Workspace creation semantics

- Creating the dedicated `Improve Kandev` workspace is an **opt-in checkbox**
  in the dialog, offered whenever the workspace does not exist yet: in the
  intro screen, and as a gate before the create form for users who dismissed
  the intro. It defaults to checked.
- When bootstrap creates the `Improve Kandev` workspace for the **first time**
  (the find-or-create miss path with `create_workspace: true`), it
  additionally:
  - **Copies the GitHub workspace connection** (source, login, installation
    metadata, and the underlying PAT secret where applicable) from the user's
    **default workspace** — resolved the same way legacy integrations resolve
    it: the active workspace recorded in the user's settings, else the
    workspace created first, else the literal `default` id. If the source
    workspace has no GitHub connection, nothing is copied.
  - **Copies nothing else**: no other integration configurations
    (Jira/Linear/GitLab/Azure DevOps/Sentry stay unconfigured in the new
    workspace; they remain manually configurable), no automations, no
    workflow/repository rows beyond the bootstrap defaults.
- When the workspace does not exist and the checkbox is unchecked
  (`create_workspace: false`), bootstrap falls back to the request's
  `workspace_id` (the user's active workspace) and scopes the repo and hidden
  workflows there — the legacy behavior.
- Reuse path (workspace already exists): bootstrap makes no further changes —
  the GitHub connection and configuration are never re-copied or synced, and
  new improve tasks land in the dedicated workspace systematically.

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
- Migrating pre-existing improve tasks out of the workspace they were created
  in before this feature shipped; old tasks stay where they are.
- Hiding the dedicated workspace from the workspace switcher; it appears as a
  normal workspace.
