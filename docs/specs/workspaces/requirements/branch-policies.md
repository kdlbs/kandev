---
status: draft
system: workspaces
created: 2026-08-24
owners:
  - kandev
---

# Branch policies requirements

## Context

Kandev treats a local path as one repository identity. A team that uses Gitflow
therefore cannot represent `feature`, `bugfix`, `hotfix`, and `release` naming
rules by registering the same path several times. A repository currently has
one fallback worktree branch template, while task creation selects only a base
branch.

Branch policies let one repository expose named combinations of base branch,
new-branch template, and pull-request target. They are a task-creation aid, not
duplicate repositories and not a restriction on Git itself.

## Goals

- Let a repository define several understandable branch workflows.
- Keep repository settings compact and provide useful contextual guidance.
- Present policies and real branches in one task-create selector without making
  a policy look like an existing branch.
- Make a selected policy deterministic for the lifetime of a task.
- Preserve all current behavior when no policy is selected.

## Non-goals

- Enforcing Gitflow or preventing users from selecting arbitrary branches.
- Automatically routing merges between release, production, and development
  branches after a pull request is created.
- Adding the saved pull-request target to agent skills or shell environments.
- Adding policies to Quick Chat, Remote URL, Add Sources, or the post-creation
  Add Branch flow in the first release.
- Reordering policies or choosing a repository-wide default policy.
- Converting the existing repository worktree template into a policy.

## Terminology

- **Branch policy**: a named repository configuration containing a base branch,
  new-branch template, and pull-request target.
- **Raw branch**: an existing branch selected directly in the current task
  creation flow.
- **Policy snapshot**: the effective policy values copied to a task-repository
  record when the task is created.
- **Gitflow starter**: a guided action that creates the initial Feature,
  Bugfix, Hotfix, and Release policies.

## Requirements

### REQ-WORKSPACES-BRANCH-POLICIES-001: Repository policy management

Each saved repository MUST support zero or more named branch policies. A policy
MUST contain a name, base branch, and branch template. Clients MAY omit the
pull-request target; the service MUST default it to the normalized base branch
before validation and persistence. A policy MAY contain a short description.

The repository editor MUST place policy management in a disclosure section that
is collapsed on each page load. The section header MUST show the policy count.
When expanded, the section MUST show a short visible explanation. Contextual
info controls MAY add detail but MUST NOT be the only explanation of the
setting.

Policy create, edit, and delete actions MUST persist independently of the
settings page save coordinator. The UI MUST require a new repository draft to
be saved before policies can be managed.

#### Acceptance criteria

- **AC-WORKSPACES-BRANCH-POLICIES-001.1:** A repository with no policies shows a collapsed `Branch
  policies` section with count zero, and existing repository settings consume no
  additional vertical space until the user expands it.
- **AC-WORKSPACES-BRANCH-POLICIES-001.2:** Expanding the section shows visible explanatory copy and
  focusable info controls for policy fields. Help is available by hover and
  keyboard focus on fine-pointer devices and by tap on touch devices.
- **AC-WORKSPACES-BRANCH-POLICIES-001.3:** A user can create, edit, and delete a policy without using the
  settings route Save button. The list and count update only after the backend
  confirms the mutation.
- **AC-WORKSPACES-BRANCH-POLICIES-001.4:** Policy names are trimmed, 1 to 100 characters, and unique within
  a repository under case-insensitive comparison. Descriptions are optional and
  no longer than 500 characters. Branch refs and rendered templates pass the
  existing safe Git-ref validation.
- **AC-WORKSPACES-BRANCH-POLICIES-001.5:** Deleting a repository deletes its policies. Deleting or editing
  a policy does not rewrite an existing task's policy snapshot.
- **AC-WORKSPACES-BRANCH-POLICIES-001.6:** Base and pull-request-target controls list distinct local and
  remote branch refs in a searchable selector with an explicit refresh action.
  Editing a policy whose saved ref is no longer listed keeps that saved value
  visible until the user chooses a replacement.

### REQ-WORKSPACES-BRANCH-POLICIES-002: Guided Gitflow starter

An empty policy section MUST offer an `Add Gitflow policies` action. The action
MUST ask for a production branch and a development branch before it creates any
policies. It MUST create the complete starter set atomically.

#### Acceptance criteria

- **AC-WORKSPACES-BRANCH-POLICIES-002.1:** The production branch defaults to the repository default branch.
  The development branch prefers `develop` when that branch is available. Both
  values remain editable.
- **AC-WORKSPACES-BRANCH-POLICIES-002.2:** Production and development are required, valid existing branch
  choices, and must differ.
- **AC-WORKSPACES-BRANCH-POLICIES-002.3:** Confirmation creates these editable policies in one transaction:
  Feature uses development, `feature/{title}-{suffix}`, and development; Bugfix
  uses development, `bugfix/{title}-{suffix}`, and development; Hotfix uses
  production, `hotfix/{title}-{suffix}`, and production; Release uses
  development, `release/{title}-{suffix}`, and production. Each tuple is base,
  template, and pull-request target.
- **AC-WORKSPACES-BRANCH-POLICIES-002.4:** The starter is rejected without partial writes when the
  repository already has a policy or another request creates one concurrently.
- **AC-WORKSPACES-BRANCH-POLICIES-002.5:** Starter policy names are persisted configuration values. Their
  field guidance and built-in descriptions are localized presentation copy.

### REQ-WORKSPACES-BRANCH-POLICIES-003: Task-create policy selection

New Task and New Subtask MUST present configured policies and existing branches
through the current repository and base-branch selector. The selector MUST use
separate `Branch policies` and `Branches` groups and MUST keep policies visually
distinct from branches.

Selecting a policy means "start from this base branch and create a new branch
with this template." It MUST NOT be treated as the name of an existing branch.

#### Acceptance criteria

- **AC-WORKSPACES-BRANCH-POLICIES-003.1:** Policy options appear before raw branches and use a single-line
  row with a localized `Policy` marker, policy name, and focusable information
  control. The information control exposes the base branch, branch-template
  preview, pull-request target, and unavailable-base guidance by hover, keyboard
  focus, or tap.
- **AC-WORKSPACES-BRANCH-POLICIES-003.2:** A selected policy remains visibly identifiable as a policy in the
  closed chip, including its base branch. Selection state uses a typed policy ID
  and is never inferred from a label or branch string.
- **AC-WORKSPACES-BRANCH-POLICIES-003.3:** Selecting a policy explicitly turns on the existing `Fork a new
  branch` mode for a local executor and keeps the state visible. Existing dirty
  working-tree consent continues to apply.
- **AC-WORKSPACES-BRANCH-POLICIES-003.4:** Selecting a raw branch preserves current checkout and fallback
  worktree-template behavior. Repositories with no policies preserve the current
  selector behavior.
- **AC-WORKSPACES-BRANCH-POLICIES-003.5:** Policies are not offered for an unsaved local path, Remote URL,
  Quick Chat, Add Sources, or post-creation Add Branch in the first release.
- **AC-WORKSPACES-BRANCH-POLICIES-003.6:** If a policy's base branch is known to be absent after a branch
  refresh, the option is unavailable and explains how to repair the policy.

### REQ-WORKSPACES-BRANCH-POLICIES-004: Immutable task application

The backend MUST resolve a selected policy against the selected repository and
authorized workspace when it creates the task. It MUST snapshot the policy
identity, name, base branch, branch template, and pull-request target on the
task-repository record.

#### Acceptance criteria

- **AC-WORKSPACES-BRANCH-POLICIES-004.1:** The task-create request sends only the selected policy ID as new
  policy input. The backend rejects a missing, deleted, cross-repository, or
  unauthorized policy and creates no partial task.
- **AC-WORKSPACES-BRANCH-POLICIES-004.2:** Policy changes and deletion after task creation do not alter the
  task's base, generated branch naming, title-triggered branch rename, or default
  pull-request target.
- **AC-WORKSPACES-BRANCH-POLICIES-004.3:** Worktree creation, local fresh-branch creation, and generated-title
  branch rename use the snapshot template. Older tasks and raw-branch tasks fall
  back to the repository worktree template.
- **AC-WORKSPACES-BRANCH-POLICIES-004.4:** Desktop and mobile pull-request creation default to the snapshot
  pull-request target and fall back to the task base branch when no target was
  snapshotted.
- **AC-WORKSPACES-BRANCH-POLICIES-004.5:** A stale policy selected in an open dialog produces an actionable
  error and refreshes available policies; Kandev does not silently substitute a
  raw branch or another policy.
- **AC-WORKSPACES-BRANCH-POLICIES-004.6:** When Kandev starts or resets an agent for a policy-backed task,
  the agent context identifies each repository's snapshotted pull-request
  target. The context instructs the agent to pass that target explicitly to a
  provider CLI. Passthrough agents receive the same instruction as plain text.
  Raw-branch tasks receive no policy-target instruction.

### REQ-WORKSPACES-BRANCH-POLICIES-005: Responsive, accessible compatibility

Branch policy settings and selection MUST remain usable with keyboard, pointer,
and touch input and MUST preserve existing repository and task contracts.

#### Acceptance criteria

- **AC-WORKSPACES-BRANCH-POLICIES-005.1:** Desktop uses the compact repository disclosure and modal form.
  Phone layouts use the same disclosure and a full-height drawer form with one
  scroll owner, a fixed safe-area action region, and touch targets of at least
  44 CSS pixels.
- **AC-WORKSPACES-BRANCH-POLICIES-005.2:** Help content has equivalent hover, focus, and tap access. A touch
  user is never required to emulate hover.
- **AC-WORKSPACES-BRANCH-POLICIES-005.3:** Policy forms, grouped selectors, help triggers, errors, and
  confirmations expose accessible names and keyboard behavior. All user-facing
  copy is localized in the supported catalogs.
- **AC-WORKSPACES-BRANCH-POLICIES-005.4:** Existing repositories require no migration action, existing task
  payloads remain valid, and tasks created without a policy continue to use the
  current repository worktree template and task base branch.

## Operational requirements

- Mutations MUST emit structured logs with workspace, repository, policy, and
  action identifiers. Task creation MUST log whether a policy was resolved or
  fallback behavior was used.
- No new metrics are required for the first release.
- Policy API authorization MUST use the same workspace and repository membership
  checks as repository settings. A cross-workspace policy identifier MUST not
  disclose policy data.

## Traceability

- System design: [Branch policies](../system-design/branch-policies.md)
- Decision: [Snapshot task branch policies](../../../decisions/2026-08-24-task-snapshotted-branch-policies.md)
- Implementation plan: [Branch policies plan](../../../plans/branch-policies/plan.md)
