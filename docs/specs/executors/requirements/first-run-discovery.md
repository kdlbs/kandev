---
status: active
system: executors
created: 2026-09-25
owners:
  - kandev
---

# First-run executor discovery requirements

## Overview

The first-run tour must explain where an agent can run before a user starts a task. The executor system owns the supported choices and their setup and trust boundaries. The tour presents those facts without changing executor configuration or task defaults.

## Requirements

### REQ-EXECUTORS-ONBOARDING-001: Accurate executor choices

**Intent:** Help a new user choose a suitable executor profile without implying that every environment is configured or isolated in the same way.

#### Acceptance criteria

- **AC-EXECUTORS-ONBOARDING-001.1:** When the user reaches the executor step, the tour shall show Local, Worktree, Local Docker, Sprites.dev, SSH, and Kubernetes. It shall not offer Remote Docker or test-only executor types.
- **AC-EXECUTORS-ONBOARDING-001.2:** The tour shall recommend Worktree for a first task on an existing Git repository. It shall explain that Worktree uses a separate checkout but shares the host account. It shall explain that Local runs in the selected folder on the Kandev host with access through that host account.
- **AC-EXECUTORS-ONBOARDING-001.3:** The tour shall distinguish a built-in executor from one that needs a Docker daemon, provider account, SSH host, or administrator-managed cluster. Prerequisite text shall not claim that a live connection or profile is ready.
- **AC-EXECUTORS-ONBOARDING-001.4:** The Docker description shall not promise full isolation. The tour shall explain that the executor controls where work runs and that a profile supplies its reusable configuration.
- **AC-EXECUTORS-ONBOARDING-001.5:** The step shall tell the user where to configure executor profiles and when a task uses one. It shall offer a link to the executor guide. Reading the step or following that guide shall not select an executor, change a profile, or create a task.
- **AC-EXECUTORS-ONBOARDING-001.6:** When the tour is available, the choices shall appear as a compact two-column card grid. The content shall remain readable with long translated text and keep navigation controls visible.
- **AC-EXECUTORS-ONBOARDING-001.7:** Cards shall read as information, not selectable options. Labels, descriptions, prerequisites, and guidance shall follow the active locale. Links and navigation shall have accessible names.
- **AC-EXECUTORS-ONBOARDING-001.8:** The executor step shall retain the tour's current Back, Next, and Skip behavior. Advancing from the agent step shall still save dirty agent-profile edits. Skip shall still set only the browser-local completion marker.
- **AC-EXECUTORS-ONBOARDING-001.9:** If a profile is replaced or removed while the tour is hidden, reopening shall use the refreshed profile settings and shall not submit dirty edits to the stale profile ID.
- **AC-EXECUTORS-ONBOARDING-001.10:** Profile saves during tour progression shall be single-flight. Pending saves shall disable navigation controls; an unexpected save failure shall show a localized error and keep the current step open for retry.

## Related requirements

The [first-run dialog availability contract](../../ui/requirements/first-run-dialog-availability.md) keeps the whole tour off phones. The [dialog-content containment contract](../../ui/requirements/dialog-content-containment.md) defines reusable viewport and scroll behavior where the dialog appears. Task creation retains ownership of executor selection and its [source-dependent default](../../tasks/requirements/task-create-executor-default.md).

## Out of scope

- New executor runtimes, profiles, connection checks, or runtime status badges.
- Changes to task creation, its executor defaults, or the content and logic of other tour steps.
- A new global onboarding step or a new cross-feature onboarding framework.
