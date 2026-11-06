---
created: 2026-11-03
status: done
requirements:
  - REQ-TASKS-MULTI-BRANCH-001
system_design:
  - ../../specs/tasks/system-design/multi-branch-live-commits.md
legacy_specs: []
---

# Repository-scoped live commits

## Scope and authorization

The preceding diagnostic turn proved that two repository commits sharing a
parent SHA collapse into one frontend row. Both WebSocket events arrive.
Waiting 31 seconds does not restore the row. A PR can supply independent files,
and a full reload restores both local commits.

The user then explicitly requested this scoped fix and a PR. This work implements
that continuation in the same session without delegation.

## Technical approach

Select the leading row within the incoming commit's normalized repository scope
for amend detection. Preserve duplicate handling, same-repository amendments, root-name
compatibility, environment ownership, and snapshot replacement.

The [system design](../../specs/tasks/system-design/multi-branch-live-commits.md)
maps `AC-TASKS-MULTI-BRANCH-001.2` to this correction.

## Verification and viewport assessment

The work order owns focused store/handler tests, lint, typecheck, specification
validation, and one real two-repository desktop/phone browser scenario.
The browser test also captures fresh synthetic PR screenshots when requested.
This state correction changes no rendered composition or interaction contract.
The existing Changes surfaces remain the desktop and phone exemplars.

Public docs need no change: the Git operations how-to already describes the
intended repository-scoped Changes workflow. The patch restores that workflow
without changing instructions, commands, terminology, or public APIs.

## Work orders

- [x] [Task 01: Preserve repository-qualified live commits](task-01-preserve-live-commits.md)
      (`done`, sequential).

## Risks and exclusions

The main compatibility risk is changing same-repository amend behavior.
Focused positive controls protect it. No provider policy, refresh mechanism,
checkout mutation, persistence, or UI redesign belongs to this repair.
The original reported task's shared-parent condition remains unverified.

## Results

RED: five new store cases failed while ten controls passed. The browser
regression received both commit events and failed on the missing desktop row.

GREEN: all 38 focused store/handler tests passed. Changed-file ESLint and
Prettier checks passed. Typecheck passed with an 8 GiB Node heap after the
default 4 GiB heap exhausted memory. No type errors remained.

The rebuilt browser scenario passed. Its final capture run passed in 9.7 s,
with both desktop and Pixel 5 views receiving both live commits. Two fresh
synthetic screenshots were inspected and compressed. The desktop capture closes
the terminal before capture to exclude the host username.

Documentation catalog validation, specification lint, the 36 specification
linter tests, and `git diff --check` passed. PR publication follows this record.

PR review also identified an amendment arriving after a newer sibling commit.
Its new regression failed before selecting the leading row within each
repository, then passed with the correction. The affected tests, lint, browser
scenario, and typecheck passed again. The PR documentation evaluator also passed
after the work order declared its sibling plan, dependency list, and wave.
The final test uses the shared `watchWs` decoder and arms both repository waits
before creating commits. The acceptance criterion is limited to repository
isolation; the existing branch-to-scope mapping remains unchanged.
