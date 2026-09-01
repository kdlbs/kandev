---
status: active
system: workspaces
created: 2026-08-17
owners:
  - kandev
---

# Repository Sets Requirements

## Overview

A workspace can contain many repositories. Most tasks use a known subset of
those repositories. A repository set stores that reusable selection and its
order.

Some teams also start work from different base branches in each repository.
The set can store an optional base-branch preference for each member. This
preference reduces repeated task setup without changing repository defaults.

The workspace system owns this contract because it owns repository membership,
repository settings, and repository branch information.

## Terminology

- **Repository set:** A named and ordered group of repositories in one
  workspace.
- **Set member:** One repository and its position in a repository set.
- **Saved base:** An optional base branch on a set member.
- **Task default:** The existing task-form branch selection when a set has no
  saved base.
- **Repository default:** The current `default_branch` of a repository.

## Requirements

### REQ-WORKSPACES-REPOSITORY-SETS-001: Repository set management

**Intent:** Users can reuse a known repository selection when they create a
task.

#### Acceptance criteria

- **AC-WORKSPACES-REPOSITORY-SETS-001.1:** A workspace shall own any number of
  repository sets. Each set shall have a name, an optional description, and an
  ordered list of repositories from that workspace.
- **AC-WORKSPACES-REPOSITORY-SETS-001.2:** The repository settings page shall
  list each set with its name, description, and members. Users shall be able to
  create, rename, reorder, edit, and delete sets.
- **AC-WORKSPACES-REPOSITORY-SETS-001.3:** A set name shall contain 1 to 100
  characters after trimming. Names shall be unique in a workspace under a
  case-insensitive comparison.
- **AC-WORKSPACES-REPOSITORY-SETS-001.4:** A set shall contain each repository
  at most once. A user-created set shall contain at least one repository.
- **AC-WORKSPACES-REPOSITORY-SETS-001.5:** When repository deletion removes the
  last member, the system shall keep the empty set.
- **AC-WORKSPACES-REPOSITORY-SETS-001.6:** Every user with workspace access
  shall see the same sets. Workspace write permissions shall control set
  mutations.
- **AC-WORKSPACES-REPOSITORY-SETS-001.7:** A mutation shall update other open
  clients through the repository-set event stream.
- **AC-WORKSPACES-REPOSITORY-SETS-001.8:** Set writes shall be atomic. A
  rejected name, member, order, or saved base shall not cause a partial write.

### REQ-WORKSPACES-REPOSITORY-SETS-002: Saved base branches

**Intent:** Users can reuse the base branch for each repository in a set.

**User story:** As a user, I want to save each repository base branch, so that I
do not select every branch for each task.

#### Acceptance criteria

- **AC-WORKSPACES-REPOSITORY-SETS-002.1:** Each set member shall store zero or
  one saved base. A member without a saved base shall use normal task-form
  defaulting when the set is applied.
- **AC-WORKSPACES-REPOSITORY-SETS-002.2:** The set editor shall show selected
  members as searchable, ordered rows. Each row shall show its repository and a
  base-branch selector.
- **AC-WORKSPACES-REPOSITORY-SETS-002.3:** The selector shall include a
  `Task default` choice. The choice shall show the current repository default
  as supporting context when that value is available.
- **AC-WORKSPACES-REPOSITORY-SETS-002.4:** A reset action shall remove all saved
  bases from the draft. The action shall not remove or reorder members.
- **AC-WORKSPACES-REPOSITORY-SETS-002.5:** Branch lists shall load when a user
  opens a selector. Opening a set with many members shall not start one branch
  request for every member.
- **AC-WORKSPACES-REPOSITORY-SETS-002.6:** A saved base that is no longer
  available shall remain visible with an unavailable state. The system shall
  not replace it with another branch.
- **AC-WORKSPACES-REPOSITORY-SETS-002.7:** The desktop editor shall use a modal
  surface. The phone editor shall use a full-height surface with one scroll
  region and a safe-area action footer.
- **AC-WORKSPACES-REPOSITORY-SETS-002.8:** All required actions shall support
  keyboard, pointer, and touch input. Touch targets shall be at least 44 CSS
  pixels in their active dimension.
- **AC-WORKSPACES-REPOSITORY-SETS-002.9:** The base control shall use the same
  selector behavior as New Task. It shall include branch search, refresh,
  remote-qualified names, and local or remote-origin labels.

### REQ-WORKSPACES-REPOSITORY-SETS-003: Task form application

**Intent:** Applying a set fills the task form without removing user work.

#### Acceptance criteria

- **AC-WORKSPACES-REPOSITORY-SETS-003.1:** New Task and New Subtask shall show a
  `Sets` control next to the repository action. Each option shall show the set
  name and member count.
- **AC-WORKSPACES-REPOSITORY-SETS-003.2:** Applying a set shall add one row for
  each available member, in set order. The action shall consume one empty
  placeholder row.
- **AC-WORKSPACES-REPOSITORY-SETS-003.3:** Applying a set shall skip a repository
  that is already in the form. The action shall not change or reorder that
  existing row.
- **AC-WORKSPACES-REPOSITORY-SETS-003.4:** Applying a set shall copy each saved
  base into its new task row. A member without a saved base shall use normal
  task-form defaulting.
- **AC-WORKSPACES-REPOSITORY-SETS-003.5:** The task form shall show an unavailable
  copied base. The user shall select an available base or the task default
  before task creation.
- **AC-WORKSPACES-REPOSITORY-SETS-003.6:** Local execution shall keep the saved
  base separate from the checkout branch. Applying a set shall not silently use
  the saved base as the local checkout branch.
- **AC-WORKSPACES-REPOSITORY-SETS-003.7:** Applying a set shall change only the
  task draft. Later set changes shall not change existing tasks or open drafts.
- **AC-WORKSPACES-REPOSITORY-SETS-003.8:** When members are missing, the form
  shall skip them and show the number skipped. Applying the same set twice
  shall not add duplicate rows.
- **AC-WORKSPACES-REPOSITORY-SETS-003.9:** `Save as set` shall capture the
  workspace repositories and effective bases shown in the task form. The dialog
  shall state that it saves base choices. A `Task default` choice shall remain
  an empty saved base.
- **AC-WORKSPACES-REPOSITORY-SETS-003.10:** The control shall remain absent from
  Quick Chat, Remote URL, and No repository modes. Executor capability shall not
  hide or disable the control.

## Compatibility

- Existing sets shall load with no saved bases and shall preserve current task
  defaulting.
- Existing create and update clients that send ordered `repository_ids` shall
  remain valid. Those members shall have no saved bases.
- Deleting a set shall not change its repositories or tasks that used it.

## Out of scope

- A shared branch value that is applied to every repository.
- Branch policies, branch templates, pull-request targets, agent profiles,
  executor profiles, or workflows in a set.
- Remote-URL sources, folder sources, cross-workspace sets, and per-user sets.
- Automatic repair of unavailable saved bases.

## Traceability

- System design: [Repository sets](../system-design/repository-sets.md)
- Implementation plan: [Repository set base branches](../../../plans/repository-set-base-branches/plan.md)
