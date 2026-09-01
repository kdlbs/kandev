---
id: "12-lifecycle-public-docs"
title: "Lifecycle public documentation"
status: done
wave: 7
depends_on:
  - "09-backend-lifecycle-reliability"
  - "10-frontend-lifecycle-feedback"
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 12: Lifecycle public documentation

## Acceptance

- Public GitHub integration docs explain the task-level lifecycle switches,
  connected-account matching, one-minute polling, existing-session delivery,
  task-level all-linked-PR scope, and immutable server-owned lifecycle prompts.
- Cleanup docs explain Auto retention for enabled lifecycle prompts and the
  explicit Always delete override.
- Feature-status terminology says review requested rather than re-review and
  keeps GitLab lifecycle parity out of scope.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/integrations.md`
- `docs/public/feature-status.md`
- This task file

## Dependencies

- Tasks 09 and 10.

## Inputs

- Approved spec and ADR-0051.
- Plan: `Required PR handoff notes`.
- Existing GitHub/GitLab integration sections in `docs/public/integrations.md`.

## Constraints

- Keep documentation task-oriented and explicit about GitHub-only scope.
- Do not imply per-workspace GitHub credentials; authentication is currently
  installation-wide even though repository scope, watches, tasks, and
  subscriptions remain workspace/task-associated.
- Do not claim lifecycle prompt editing or overrides exist in the UI, HTTP,
  MCP, or storage.
- Do not edit `plan.md`; update only this task file's status.

## Output contract

- Public docs summary and exact files changed.
- Validation commands/results.
- Exact PR-body handoff note covering immutable lifecycle prompt text and the
  remaining named follow-ups.
- Blockers, divergence, and follow-up risks.
- Set this task file to `done` only after acceptance and validation pass.
