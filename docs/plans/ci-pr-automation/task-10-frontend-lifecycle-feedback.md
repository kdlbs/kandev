---
id: "10-frontend-lifecycle-feedback"
title: "Frontend lifecycle feedback"
status: done
wave: 6
depends_on: ["08-pr-lifecycle-agent-prompts"]
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 10: Frontend lifecycle feedback

## Acceptance

- The shared automation controls label the switch `Your review is requested`,
  explain that new requests include both initial review and re-review, group
  the separate merged/closed switches under their shared value, and do not
  expose lifecycle prompt editing.
- The selected linked PR's non-empty `last_error` renders as a compact,
  accessible error row shared by the desktop popover and mobile drawer.
- GitHub Review Watch cleanup copy explains that Auto retains tasks with user
  engagement or enabled lifecycle prompts and that Always delete overrides
  retention.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- pr-ci-popover.automation.test.tsx review-watch-dialog.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/github/pr-ci-automation-controls.tsx`
- `apps/web/components/github/pr-ci-popover.automation.test.tsx`
- `apps/web/components/github/review-watch-dialog.tsx`
- `apps/web/components/github/review-watch-dialog.test.tsx`
- This task file

## Dependencies

- Task 08 and the approved lifecycle requirements in the spec.

## Inputs

- Spec: lifecycle UI behaviors, failure modes, scenarios, and out-of-scope
  prompt editors.
- Plan: `Lifecycle labels, errors, and cleanup copy` plus the mobile design
  contract.
- Existing patterns: `CIAutomationErrorRow`,
  `findCIAutomationStateForPR`, `PRStatusChipDrawer`, and
  `CLEANUP_POLICY_OPTIONS`.

## Constraints

- Use TDD and `@kandev/ui` primitives.
- Reuse `PRCIAutomationControls`; do not fork desktop/mobile business logic.
- Do not add a lifecycle prompt editor, nested scroll owner, or new mobile
  overlay.
- Preserve the existing auto-fix prompt editor.
- Do not edit `plan.md`; update only this task file's status.

## Output contract

- Summary of behavior and mobile-parity result.
- Files changed.
- Tests/typecheck run with exact results.
- Blockers, divergence, and follow-up risks.
- Set this task file to `done` only after acceptance and targeted verification
  pass.
