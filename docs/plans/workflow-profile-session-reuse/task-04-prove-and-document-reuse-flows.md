---
id: "04-prove-and-document-reuse-flows"
title: "Prove and document reuse flows"
status: done
wave: 4
depends_on:
  - "03-expose-workflow-policy"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.2
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.3
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.7
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 04: Prove and Document Reuse Flows

## Summary

Prove the policy from the workflow editor through runtime session identity on
desktop and through touch-safe save/reload on mobile. Document the three choices
and their session-count and conversation-continuity trade-offs.

## In scope

- Desktop Playwright policy and `A -> B -> A` runtime scenarios.
- Mobile Playwright policy persistence, hit-area, and overflow scenario.
- Public Tasks and Workflows how-to update and docs validation.

## Out of scope

- Broad E2E or documentation-site builds outside the changed page.
- New product media.

## Acceptance

- Desktop UI selection persists and runtime IDs prove reuse versus fresh
  profile re-entry.
- Mobile touch selection persists after reload, remains viewport-contained, and
  has no document horizontal overflow.
- Public documentation states the default and when to choose each policy.

## Verification

```bash
(cd apps/web && pnpm e2e:run tests/workflow/workflow-agent-switch.spec.ts -- --grep 'profile session policy')
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/workflow/mobile-workflow-settings.spec.ts -- --grep 'profile session policy')
node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/e2e/pages/workflow-settings-page.ts`
- `apps/web/e2e/tests/workflow/workflow-agent-switch.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`
- `docs/public/tasks-and-workflows.md`

## Dependencies

Task 03.

## Risks

- The desktop scenario must wait on observable session state and identity, not a
  fixed delay.
- The mobile run may use `--no-build` only after the desktop managed runner has
  built the current backend and frontend artifacts.

## Parallelism

`sequential`

## Inputs

- Requirement AC 001.2, 001.3, 001.6, and 001.7.
- Existing workflow-agent-switch and mobile workflow settings fixtures.
- Docs-maintainer how-to guidance.

## Results

Added desktop coverage for policy save and reload, `A -> B -> A` reuse with
`park_reuse`, and fresh-session re-entry with `park_new`. Added mobile coverage
for touch selection, 44px hit areas, save and reload, and document overflow.
Updated `docs/public/tasks-and-workflows.md` with the default, all three policy
choices, runtime behavior, and selection guidance.

Verification:

- Desktop profile-session E2E passed: 2 tests.
- Mobile profile-session E2E passed: 1 test.
- `node --test scripts/validate-public-docs.test.mjs` passed: 61 tests.
- `node scripts/validate-public-docs.mjs` passed: 41 published pages validated.
