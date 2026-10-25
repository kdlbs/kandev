---
id: "20-role-aware-automation-controls"
title: "PR events automation controls"
status: done
wave: 12
depends_on:
  - "19-final-review-remediation"
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 20: PR events automation controls

## Intent

Keep every task-wide PR automation option usable while reducing primary-list
noise by grouping the three agent lifecycle prompt switches together.

## Owned files

- `apps/web/components/github/pr-ci-automation-controls.tsx`
- `apps/web/components/github/pr-ci-popover.automation.test.tsx`
- `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
- `apps/web/e2e/tests/pr/mobile-ci-automation-options.spec.ts`

## Acceptance

- Auto-fix and auto-merge remain in the primary automation list for every task.
- Review-request, merged, and closed prompt switches are reachable together
  under a `PR events` disclosure for every task.
- The disclosure is presentation only and does not mutate stored options.
- A disclosure containing any enabled option opens so active automation is not
  concealed.
- The same shared behavior is usable in the desktop popover and mobile drawer,
  with the existing touch-sized rows preserved.
- Focused component coverage and desktop/mobile Playwright coverage exercise
  disclosure reachability, enabled-state opening, option updates, and existing
  touch sizing/overflow behavior.

## Verification

```bash
cd apps/web && pnpm vitest run components/github/pr-ci-popover.automation.test.tsx
cd apps/web && pnpm e2e:run tests/pr/ci-automation-options.spec.ts tests/pr/mobile-ci-automation-options.spec.ts
```

## Risks

- Options remain task-wide even though the disclosure is rendered inside a
  selected PR's popover or drawer.
- Active lifecycle automation must not be silently concealed when the surface
  opens.
