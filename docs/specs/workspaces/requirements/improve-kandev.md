---
status: active
system: workspaces
created: 2026-04-29
updated: 2026-11-19
owners:
  - Carlos Florencio
---
# Improve Kandev Requirements

## Overview

Users who hit a bug or have a feature idea today have no in-app way to report it, and even when they do, the report sits as text someone else has to act on. Make filing an improvement a one-click action that produces a real, actionable task the user's own agent picks up immediately — turning every report into a contribution.

## Requirements

### REQ-WORKSPACES-IMPROVE-KANDEV-001: Improve Kandev

**Intent:** Users who hit a bug or have a feature idea today have no in-app way to report it, and even when they do, the report sits as text someone else has to act on. Make filing an improvement a one-click action that produces a real, actionable task the user's own agent picks up immediately — turning every report into a contribution.

#### Acceptance criteria

- **AC-WORKSPACES-IMPROVE-KANDEV-001.1:** The **Improve Kandev** action in the desktop app-sidebar footer opens a task-creation dialog that is pre-configured for the kandev codebase: repository locked to `https://github.com/kdlbs/kandev`, base branch `main`, workflow selected from the hidden Improve Kandev workflows, and description seeded with a starter template. On phones the same action is a 44px-or-larger row in the existing mobile home menu's **Utilities** section.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.2:** The dialog reuses the existing task-create UI, including prompt enhancement, image paste, and file attachments.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.3:** The dialog explains the flow up front: the agent will implement the change, the user will test it, then the agent opens a PR. Brief copy positions this as the user contributing to kandev's future.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.4:** The explanation includes a "Do not show this again" preference. Once selected, later uses of **Improve Kandev** skip the explanation and open the pre-configured task-creation dialog directly. The preference is local to the current browser profile and can be cleared with other local UI state.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.5:** The task-creation dialog offers three report kinds: **Bug fix**, **Feature request**, and **Open issue**. Bug fixes and feature requests use the existing implementation workflow. Open issue uses a separate hidden, one-step workflow and visibly explains that the agent only publishes a GitHub issue; it does not implement the change or open a pull request.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.6:** An "Include recent logs" toggle (default on) attaches a context bundle to the task: recent backend logs, frontend logs, and a metadata snapshot. The bundle lives in a temporary folder and is referenced by file path in the task description so the agent can read it on demand.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.7:** Submitting the dialog creates the task in the dedicated **Improve Kandev** workspace, clones the kandev repo if needed, and starts the agent on the first step.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.8:** The dedicated workspace is created automatically on first bootstrap and reused on every later use, keeping improve tasks isolated and segregated from the user's regular work. It is named `Improve Kandev`, is a normal visible workspace (with a kanban workflow), and persists across restarts.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.9:** When bootstrap creates either managed workflow, it shall provision every step from the selected system template before it reports success. The workspace's user-facing read-only restriction shall not block this initialization.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.10:** When a managed workflow has zero steps, the next bootstrap shall restore its template steps and retain its workflow identity. A workflow with at least one step shall keep its existing step identities and configuration. This rule applies independently to both workflows, including mixed empty and populated states. Repeated or concurrent bootstrap calls shall not duplicate steps.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.11:** When required step initialization fails, bootstrap shall report an error and keep the task form blocked. It shall not launch an agent or report an empty workflow as ready. A later bootstrap shall retry an empty workflow without manual data repair.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.12:** User-facing workflow and step mutations in the dedicated workspace shall remain blocked. Bootstrap shall retain workspace authorization and executor eligibility restrictions. It shall not modify workflows outside the caller's authorized scope or workflows managed by GitHub sync.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.13:** When a workflow has no steps, the workflow steps API shall return an empty array. On desktop and phones, the client shall accept a legacy null or missing step list as an empty list without an exception. Populated step lists shall retain their existing normalization.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.14** When the dialog requests preparation,
  the server shall acknowledge the request independently of repository checkout
  and GitHub probes. The desktop and phone dialogs shall show progress. Task
  creation shall stay disabled until the repository and both initialized
  workflows are available.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.15** When preparation fails or its status
  is unavailable, the desktop and phone dialogs shall show an error and offer
  **Retry**. A retry in the open dialog shall keep the report draft and
  workspace choice.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.16** When the dialog closes or its workspace
  context changes, the earlier attempt shall stop updating that dialog.
  Preparation can finish after the dialog closes. After a backend restart,
  a new attempt shall reuse completed setup and prepare any missing checkout.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.17** When two attempts target the same
  workspace, they shall converge on one repository and one workflow per template.
  Each successful attempt shall have its own diagnostic directory. Only the
  initiating account shall be able to read its preparation status.
- **AC-WORKSPACES-IMPROVE-KANDEV-001.18** Calls that use the existing synchronous
  bootstrap contract shall receive the existing response shape and status
  semantics. The dialog shall accept the completed response from a server that
  supports only that contract.

## Out of scope

- Replacing or completing a populated workflow from a newer template.
- Changing the user-facing read-only policy, task launch policy, or GitHub credential policy.
- Moving existing tasks, replacing workspace connections, or adding a database migration.

## System design

See the [Improve Kandev design](../system-design/improve-kandev.md) and its
[bootstrap lifecycle](../system-design/improve-kandev-bootstrap.md).
