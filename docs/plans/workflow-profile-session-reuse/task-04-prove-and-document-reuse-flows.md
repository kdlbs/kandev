---
id: "04-prove-and-document-reuse-flows"
title: "Prove mixed step policies and document behavior"
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
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.10
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 04: Prove Mixed Step Policies and Document Behavior

## Summary

Prove per-step policy from the combined selector through runtime session
identity on desktop and through touch-safe save and reload on mobile. Document
destination-step ownership and conversation-continuity trade-offs.

## In scope

- Desktop Playwright mixed-step save/reload and `A -> B -> A` runtime
  scenarios.
- Mobile Playwright profile search, nested policy navigation, persistence,
  focus, hit-area, safe-area, and overflow scenarios.
- Public Tasks and Workflows how-to update and docs validation.

## Out of scope

- Broad E2E or documentation-site builds outside the changed page.
- New product media.

## Acceptance

- Two steps save and reload different policy values.
- Desktop runtime IDs prove reuse versus fresh entry from the destination step's
  setting.
- Mobile selection persists after reload, returns focus correctly, remains
  viewport-contained, and has no document horizontal overflow.
- Public documentation states the per-step default and when to choose each
  policy.

## Verification

```bash
(cd apps/web && pnpm e2e:run e2e/tests/workflow/workflow-agent-switch.spec.ts -- --grep 'profile session policy')
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome e2e/tests/workflow/mobile-workflow-settings.spec.ts -- --grep 'profile session')
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
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
- The test must assign policies to destination steps explicitly. A workflow-wide
  fixture would fail to prove the ownership change.
- The mobile run may use `--no-build` only after the desktop managed runner has
  built the current backend and frontend artifacts.

## Parallelism

`sequential`

## Inputs

- Requirement AC 001.2, 001.3, 001.6, 001.7, 001.10, and 001.11.
- Existing workflow-agent-switch and mobile workflow settings fixtures.
- Docs-maintainer how-to guidance.

## Results

Added desktop save/reload coverage and `A -> B -> A` identity assertions for
both `park_reuse` and `park_new`. Added mobile touch coverage for profile search,
nested policy selection, 44px hit areas, save/reload, focus return, safe-area
containment, and document overflow. Updated public workflow guidance and
validated 61 documentation tests across 41 published pages. Both desktop
policy tests and the targeted mobile selector tests passed.
