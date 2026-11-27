---
id: "02-update-guidance"
title: "Update public guidance and delivery records"
status: done
wave: 2
depends_on:
  - "01-remove-task-controls"
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-004
acceptance_criteria:
  - AC-TASKS-COMPLETION-004.1
  - AC-TASKS-COMPLETION-004.3
  - AC-TASKS-COMPLETION-004.4
system_design:
  - ../../specs/tasks/system-design/coordination-controls.md
---

# Task 02: Update public guidance and delivery records

## Summary

Remove public instructions for controls that no longer exist. Record the final
implementation evidence without overwriting historical #3994 results.

## In scope

- Correct the native task-detail claims in `docs/public/plugins-authoring.md`.
- Explain retained task APIs and the absence of built-in inspection/recovery dialogs.
- Check README and screenshot catalog references; change them only if affected.
- Record actual work-order commands and outcomes; reconcile prior plan annotations.
- Promote the paired draft specs only if the completed implementation satisfies their full contracts. Do not promote unrelated unfinished requirements in a shared document.

## Out of scope

No new UI, SDK changes, plugin promises, or retrospective changes to historical test counts.
The public authoring page remains a reference page.

## Acceptance

1. Public guidance no longer directs users to native Manager/Inspect dialogs and accurately states the surviving API and consent boundary.
2. Plans distinguish historical UI evidence from the replacement coverage, with actual results and accurate statuses.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/public docs/specs docs/plans
git status --short -- docs/plans/remove-native-coordination-ui
```

Search `docs/public`, `README.md`, and `docs/screenshots.md` for the removed
control labels. Confirm any surviving mention describes an API or historical
context. Run the local documentation-coverage preflight against the changed
work orders and their linked plan/design/requirements.

## Files likely touched

- `docs/public/plugins-authoring.md`
- `docs/plans/remove-native-coordination-ui/plan.md` and both work orders
- `docs/plans/plugin-coordinator-platform/plan.md`, `task-09-task-claims.md`, `task-10-completion-gates.md`, and `task-17-reference-operations.md`
- `docs/specs/tasks/requirements/task-completion.md`
- `docs/specs/tasks/system-design/coordination-controls.md`

## Dependencies

Task 01 must pass before documentation describes removal as shipped.

## Risks

Do not claim a plugin supplies recovery controls without verifying that plugin.
Public docs must not instruct plugins to use private HTTP APIs for human actions.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), [audit](pr-3994-ui-audit.md), and task 01 results
- Existing authoring sections on task management claims and completion gates
- [Requirement](../../specs/tasks/requirements/task-completion.md) and [design](../../specs/tasks/system-design/coordination-controls.md)

## Results

Completed on September 28, 2026. `docs/public/plugins-authoring.md` now
describes the surviving plugin task APIs and states that human recovery remains
outside the plugin Host API. It no longer promises built-in claim history,
criteria inspection, or recovery dialogs.

The root README and `docs/screenshots.md` had no affected references. The parent
plugin-coordinator plan and work orders 09, 10, and 17 now mark the task-detail
UI-03 mockup as superseded for presentation only. Their historical results remain
intact, and the task APIs and enforcement remain in scope.

The requirement and system-design frontmatter remain `draft`. These shared
documents cover REQ-TASKS-COMPLETION-001 through -004 and related technical
contracts. This implementation completes the presentation amendment, but it
does not revalidate every contract in both documents.

Validation passed:

- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 47 published pages validated.
- `python3 scripts/list-docs.py validate`: 321 decisions and 1,220 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- The local PR documentation-coverage preflight accepted both work orders for
  the simulated production-source diff and resolved their requirement/design links.
- `git diff --check` passed for tracked and untracked changed files.
