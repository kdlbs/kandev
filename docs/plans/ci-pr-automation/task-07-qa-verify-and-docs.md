---
id: "07-qa-verify-and-docs"
title: "QA verify and docs"
status: done
wave: 9
depends_on:
  - "17-final-lifecycle-transition-coverage"
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 07: QA Verify and Docs

## Acceptance

- The feature is checked against every spec scenario and any gaps are fixed or documented.
- Relevant scoped `AGENTS.md`, specs, or decisions are updated if implementation changes durable conventions.
- Final format, typecheck, tests, and lint pass or documented blockers remain.

## Verification

```bash
make fmt
make typecheck test lint
```

## Files Likely Touched

- `docs/plans/ci-pr-automation/task-07-qa-verify-and-docs.md`
- Relevant `AGENTS.md` files only if implementation changes documented conventions
- `docs/decisions/*.md` only if a durable architecture decision emerges during implementation

## Dependencies

- Task 17.

## Inputs

- Full spec.
- Full plan.
- All completed task output contracts.
- Verification skill guidance.

## Output Contract

When complete, update this file's `status` to `done` and report changed files,
tests run, blockers, and residual risks. Do not edit `plan.md`.
