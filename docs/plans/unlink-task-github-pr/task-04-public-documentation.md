---
id: "04-public-documentation"
title: "Public documentation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/link-existing-task-github-issue.md"
---

# Task 04: Public Documentation

## Acceptance

- The public session/review guide tells desktop and mobile users where the
  multi-PR unlink action appears.
- The guide states that unlinking removes only Kandev's association and leaves
  the GitHub PR, branch, commits, task repositories, and sibling links intact.
- Public documentation validation passes without navigation or link errors.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/sessions-and-review.md`

## Dependencies

None for wording; re-check the final UI label after Task 02 before marking this
task done.

## Parallelism

Parallel-safe with Task 01 because it owns only public documentation. Execution
still defaults to sequential without explicit user authorization.

## Inputs

- Spec `What` and unlink scenarios.
- Plan `Public Documentation` section.
- Existing multi-PR selector guidance in
  `docs/public/sessions-and-review.md`.

## Risks

- Do not imply that unlinking closes the provider PR or disables GitHub
  automation globally; only the selected task association leaves the task.

## Output contract

Report the exact wording location, files changed, validation results,
remaining risks/blockers, and update this task plus `plan.md` status in the same
conversation.
