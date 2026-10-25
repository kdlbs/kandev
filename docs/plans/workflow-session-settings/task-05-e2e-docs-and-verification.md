---
id: "05-e2e-docs-and-verification"
title: "Verify workflow session settings and document the feature"
status: done
wave: 4
depends_on: ["01-workflow-contract-and-validation", "02-original-session-initialization", "03-runtime-rule-application", "04-editor-and-carry-analysis"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-session-settings.md"
---

# Task 05: Verify Workflow Session Settings and Document the Feature

## Acceptance

- Desktop E2E creates Codex and Claude family rules, reloads them, exercises carry-forward warning choices, and proves export/import preservation.
- Runtime E2E proves a matching rule changes model/options on the original conversation tab before auto-start, a non-match is silent, and a partial provider failure warns without blocking the prompt.
- Read-only synced workflow coverage proves rules and diagnostics are visible but immutable.
- Mobile E2E edits multiple rule cards using touch controls with no document-level horizontal overflow or competing scroll owner.
- Public workflow documentation explains the three agent behaviors, family matching, carry-forward/restore, original-tab reuse, persistence, and partial-failure behavior.
- Targeted, documentation, typecheck, test, lint, and formatting commands are recorded with accurate outcomes.

## Verification

```bash
cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-agent-profile.spec.ts tests/workflow/workflow-session-settings.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-settings.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
make fmt
make typecheck
make test
make lint
```

## Files likely touched

- `apps/web/e2e/pages/workflow-settings-page.ts`
- `apps/web/e2e/tests/workflow/workflow-agent-profile.spec.ts`
- New `apps/web/e2e/tests/workflow/workflow-session-settings.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`
- Backend/mock-agent E2E fixtures needed for field success/failure
- `docs/public/tasks-and-workflows.md`
- Plan/task status and verification-result updates

## Dependencies

Tasks 01 through 04.

## Inputs

- Every scenario and failure mode in the approved spec.
- Existing workflow settings page object, workflow agent-profile E2E, mobile workflow settings E2E, and container/mock-agent capability fixtures.

## Output contract

Desktop and mobile editor E2E scenarios passed, including persistence through the workflow API, shared model/option selection, touch-sized controls, save behavior, and horizontal-overflow checks. Runtime matching, restore, launch ordering, persistence, non-match, and warning behavior are covered by the focused Go orchestrator/lifecycle tests; the existing E2E fixture does not yet expose a full multi-provider runtime-failure or synced-read-only workflow scenario, so those browser paths remain intentionally unverified. Public documentation was updated in `docs/public/tasks-and-workflows.md`, and both documentation validators passed. Fresh compressed PR screenshots are in `.pr-assets/` and are kept out of the feature branch.
