---
id: "04-document-task-actions"
title: "Document Threads task actions"
status: done
wave: 4
depends_on:
  - "03-phone-task-actions"
plan: "plan.md"
requirements:
  - REQ-TASKS-THREADS-ACTIONS-001
  - REQ-TASKS-THREADS-ACTIONS-002
  - REQ-TASKS-THREADS-ACTIONS-003
  - REQ-TASKS-THREADS-ACTIONS-004
acceptance_criteria:
  - AC-TASKS-THREADS-ACTIONS-001.1
  - AC-TASKS-THREADS-ACTIONS-001.3
  - AC-TASKS-THREADS-ACTIONS-001.4
  - AC-TASKS-THREADS-ACTIONS-001.5
  - AC-TASKS-THREADS-ACTIONS-001.6
  - AC-TASKS-THREADS-ACTIONS-002.2
  - AC-TASKS-THREADS-ACTIONS-003.2
  - AC-TASKS-THREADS-ACTIONS-003.3
  - AC-TASKS-THREADS-ACTIONS-004.1
  - AC-TASKS-THREADS-ACTIONS-004.2
  - AC-TASKS-THREADS-ACTIONS-004.3
system_design:
  - ../../specs/tasks/system-design/threads-task-actions.md
---

# Task 04: Document Threads Task Actions

## Summary

Explain the completed desktop and phone task actions in the existing Threads
usage guide. Record the verified implementation boundaries and work-order
results without publishing unimplemented intent.

## In scope

- Extend the Threads how-to section of `docs/public/sessions-and-review.md`
  with header/overflow entry, nested phone choices, and the task-level target.
- Link to the existing archive/delete guidance in `tasks-and-workflows.md` for
  consequences and confirmation preference; add only a small Threads entry
  reference there if useful. Preserve both pages' parent changes.
- Update `components/threads/AGENTS.md` to name the shared action owner,
  header-only context boundary, and fallback integration.
- Reconcile this package with the final implementation, record exact prior
  test/image evidence, and update requirement/design/work-order statuses as
  appropriate after the implementation request and completed checks.

## Out of scope

- Production code, new tests, a broad verification audit, new public pages,
  screenshots of a developer instance, default Home behavior, commit/push/PR.

## Acceptance

1. The guide describes the tested desktop and phone entry points, task target,
   supported actions, and cancellation/empty behavior using visible product
   names. It links to the existing cleanup reference rather than duplicating
   its policy.
2. Scoped engineering guidance names the actual shared action and viewport
   owners. Durable design and plan results reflect the completed work, including
   parent commit provenance and new phone inspection evidence.
3. Public-doc validation and specification checks pass, with any blocker
   recorded accurately; test results are taken from completed work orders.

## Verification

Run from the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

No new TDD cycle applies to Markdown-only documentation. Do not rerun product
test suites merely to rewrite their recorded results.

## Files likely touched

- `docs/public/sessions-and-review.md` (Threads how-to).
- `docs/public/tasks-and-workflows.md` (existing cleanup reference, if needed).
- `apps/web/components/threads/AGENTS.md`.
- `docs/specs/tasks/requirements/threads-task-actions.md` and paired design.
- `docs/plans/threads-task-actions/plan.md` and completed work-order results.

## Dependencies

Tasks 01-03 and their recorded validation evidence.

## Risks

The parent also edits these public pages and Threads guidance. Preserve the
integrated text and explain only this task's additional capability.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/threads-task-actions.md).
- [Design](../../specs/tasks/system-design/threads-task-actions.md).
- `/docs-maintainer`, `docs/public/README.md`, and prior work-order results.

## Results

Completed after the desktop/mobile implementation and outcome checks.

- Updated the existing Threads section of `sessions-and-review.md`, preserving
  the parent's text. It describes header/touch entry, nested Back navigation,
  captured task identity, cancellation and deterministic survivor/empty state.
  Cleanup consequences link to the existing task/workflow guide.
- Updated the existing public coverage entry and scoped Threads guidance to
  identify the shared action/lifecycle owners and header-only event boundary.
- Reconciled the requirements, design, plan and all four work orders with the
  implemented flow and actual test/image results. Requirements are `active`,
  design is `current`, and the plan/work orders are `done`.
- The public validator's unit tests and its 46-page live validation passed.
  Specification linter tests passed (30), all specification files passed lint,
  and `git diff --check` passed. No public pages, navigation destinations,
  localization keys, default Home behavior, commit, push or PR were added.
