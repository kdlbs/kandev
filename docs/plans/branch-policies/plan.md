---
spec: docs/specs/workspaces/requirements/branch-policies.md
system_design: docs/specs/workspaces/system-design/branch-policies.md
created: 2026-08-24
status: complete
---

# Implementation plan: Repository branch policies

## Outcome

Give one canonical repository reusable branch workflows for Gitflow and similar
conventions. Users manage the policies in a compact repository-settings
disclosure and select them beside real branches when creating a task. The
backend snapshots the selected policy so later configuration edits cannot
change task behavior.

## Scope

The implementation adds repository policy persistence and APIs, atomic Gitflow
seeding, task snapshot resolution, runtime consumption, responsive repository
settings, the grouped task-create selector, desktop/mobile E2E evidence, and
public documentation.

It preserves the existing repository worktree template as the raw-branch and
legacy fallback. It does not add default policies, merge automation, policy
enforcement, policy reordering, or policy selection to Quick Chat, Remote URL,
Add Sources, or Add Branch.

## Implementation decisions

- Policy configuration is repository-owned; effective task behavior is owned by
  an immutable task-repository snapshot.
- The browser submits a policy ID. The backend resolves policy fields and
  rejects stale or cross-repository IDs atomically.
- Settings policy mutations are immediate modal CRUD and do not join the route
  save coordinator.
- Task selection is a tagged union displayed in one grouped branch selector.
- Selecting a policy visibly enables the existing local fresh-branch mode.
- The phone policy form is a drawer with touch-native help; the task selector
  remains the existing compact popover.

## Work packages

### Wave 1

- [x] [Task 01: Add branch-policy persistence and APIs](task-01-branch-policy-persistence-api.md)

### Wave 2

- [x] [Task 02: Snapshot and apply task branch policies](task-02-task-policy-snapshot-runtime.md)
- [x] [Task 03: Build repository policy settings](task-03-repository-settings-policies.md)

Task 02 and Task 03 both depend on Task 01. They are sequential by default
because they share the newly introduced transport contract and model names.

### Wave 3

- [x] [Task 04: Add policies to task creation](task-04-task-create-policy-selector.md)

### Wave 4

- [x] [Task 05: Cover branch policies in desktop and mobile E2E](task-05-branch-policy-e2e.md)
- [x] [Task 06: Document branch-policy workflows](task-06-branch-policy-public-docs.md)

Tasks 05 and 06 are parallel-safe after Task 04 because they own disjoint E2E
and public-documentation files.

## Traceability matrix

| Acceptance criteria | Work package |
| --- | --- |
| AC-WORKSPACES-BRANCH-POLICIES-001.1, AC-WORKSPACES-BRANCH-POLICIES-001.2, AC-WORKSPACES-BRANCH-POLICIES-001.3 | Task 03, Task 05 |
| AC-WORKSPACES-BRANCH-POLICIES-001.4, AC-WORKSPACES-BRANCH-POLICIES-001.5 | Task 01, Task 03 |
| AC-WORKSPACES-BRANCH-POLICIES-001.6 | Task 03, Task 05 |
| AC-WORKSPACES-BRANCH-POLICIES-002.1 through AC-WORKSPACES-BRANCH-POLICIES-002.5 | Task 01, Task 03, Task 05 |
| AC-WORKSPACES-BRANCH-POLICIES-003.1 through AC-WORKSPACES-BRANCH-POLICIES-003.6 | Task 04, Task 05 |
| AC-WORKSPACES-BRANCH-POLICIES-004.1 through AC-WORKSPACES-BRANCH-POLICIES-004.6 | Task 02, Task 04, Task 05 |
| AC-WORKSPACES-BRANCH-POLICIES-005.1 through AC-WORKSPACES-BRANCH-POLICIES-005.3 | Task 03, Task 04, Task 05 |
| AC-WORKSPACES-BRANCH-POLICIES-005.4 | Task 01, Task 02, Task 04 |

## Validation ladder

1. Run focused backend persistence and service tests after Tasks 01 and 02.
2. Run focused frontend unit tests, typecheck, and i18n checks after Tasks 03
   and 04.
3. Run the four focused desktop/mobile Playwright specs in Task 05.
4. Run the public-doc validators in Task 06.
5. Before delivery, run the standard backend and web checks appropriate to all
   changed files and confirm every task result in this plan.

## Risks

- Several runtime paths currently read the live repository template. Missing
  one would make the snapshot contract inconsistent; Task 02 inventories and
  tests worktree creation, local fresh branch, title rename, and PR defaults.
- Policy deletion must preserve task history. Snapshot provenance therefore
  cannot use a cascading foreign key.
- Local fresh-branch and direct-checkout flows share the picker. Typed state and
  explicit fork-mode transition prevent accidental branch creation.
- Tooltip-only guidance would be inaccessible on touch. Task 03 requires visible
  field help and an equivalent touch drawer.
- The five-language catalog gate makes copy part of implementation, not a
  follow-up.

## Delivery status

All six work packages are implemented. Focused backend and frontend tests,
desktop and phone browser scenarios, localization checks, public-document
validators, specification lint, and frontend lint/typecheck passed.

Review remediation also covers orphan-task prevention for remote contribution
policy mismatches, policy-snapshot preservation during ordinary unstarted-task
edits, complete policy fields in task WebSocket events, local fresh-branch
creation for policy-backed subtasks, fixed selector ordering and previews,
revision-guarded policy refreshes, stale-policy recovery, and keyboard-focused
desktop help. The branch was rechecked with the focused backend and frontend
suites, full backend tests with the repository's external config disabled, web
typecheck/lint/i18n checks, and targeted desktop/mobile Playwright scenarios.

The follow-up delivery injects each snapshotted pull-request target into agent
context on first launch and context reset. Passthrough agents receive the same
instruction as plain text. Focused orchestrator and message-handler tests cover
the stored and delivered prompt contracts. The three affected backend packages
passed 2,813 tests. Specification lint and public-document validation passed.
