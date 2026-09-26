---
id: "04-public-documentation"
title: "Public WIP documentation"
status: done
wave: 4
depends_on: ["03-conflict-adapters-and-watcher-deferral"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 04: Public WIP Documentation

## Acceptance

- Public workflow documentation states that WIP capacity applies to initial
  task creation as well as moves and transitions.
- Documentation explains that integration-created tasks rejected for capacity
  remain deferred for later retry and do not auto-start.
- Public-doc validation succeeds.

## Evidence

Updated `docs/public/tasks-and-workflows.md` and
`docs/public/workflow-tips.md`. `node scripts/validate-public-docs.test.mjs`
passed all 58 tests.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/tasks-and-workflows.md`
- `docs/public/workflow-tips.md`

## Dependencies

Task 03.

## Parallelism

`sequential`

## Inputs

- Amended WIP spec.
- Final typed conflict and review-watch deferral behavior from Tasks 01-03.

## Output contract

Mark this task `in_progress` before editing and `done` after validation. Update
`plan.md` and report the public wording changed, files changed, exact validation
results, blockers, and residual documentation risks.
