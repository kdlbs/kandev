---
id: "07-qa-verify-and-docs"
title: "QA verify and docs"
status: pending
wave: 4
depends_on: ["06-e2e-ci-automation"]
plan: "plan.md"
spec: "../../specs/ui/ci-pr-automation.md"
---

# Task 07: QA Verify and Docs

## Acceptance

- The feature is checked against every spec scenario and any gaps are fixed or documented.
- Relevant scoped `AGENTS.md`, specs, or decisions are updated if implementation changes durable conventions.
- Final format, typecheck, tests, and lint pass or documented blockers remain.

## Verification

```bash
rtk make fmt
rtk make typecheck test lint
```

## Files Likely Touched

- `docs/specs/ui/ci-pr-automation.md`
- `docs/plans/ci-pr-automation/plan.md`
- `docs/plans/ci-pr-automation/task-*.md`
- Relevant `AGENTS.md` files only if implementation changes documented conventions
- `docs/decisions/*.md` only if a durable architecture decision emerges during implementation

## Dependencies

- `06-e2e-ci-automation`

## Inputs

- Full spec.
- Full plan.
- All completed task output contracts.
- Verification skill guidance.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 4 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
