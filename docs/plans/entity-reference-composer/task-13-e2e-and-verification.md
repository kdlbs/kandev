---
id: "13-e2e-and-verification"
title: "Entity reference E2E and integrated verification"
status: completed
wave: 5
depends_on: ["12-message-reference-ui"]
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 13: Entity Reference E2E and Integrated Verification

## Acceptance

- Desktop E2E proves task chat keyboard insertion/no auto-send/explicit send, clickable persistence, partial provider results, draft restore, Quick Chat parity, and literal passthrough behavior.
- Mobile E2E proves touch selection and send, visual-viewport containment, internal scrolling, 44 px rows, and no document horizontal overflow.
- Format, typecheck, unit/integration tests, lint, focused E2E, QA, security review, and code review complete with no unresolved required issue.

## Verification

```bash
make fmt
make typecheck test lint
cd apps/web && pnpm e2e:run tests/chat/entity-reference-composer.spec.ts tests/chat/mobile-entity-reference-composer.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/chat/entity-reference-composer.spec.ts`
- `apps/web/e2e/tests/chat/mobile-entity-reference-composer.spec.ts`
- `apps/web/e2e/pages/session-page.ts` only for a small reusable helper
- provider mock controllers/fixtures only where deterministic mixed-source seeding is missing

## Dependencies

Task 12 and all integrated backend work.

## Inputs

Every spec scenario; slash composer desktop/mobile precedents; E2E backend rebuild requirements.

## Output contract

Report exact commands/results, browser artifacts for failures, desktop/mobile visual findings, QA/security/review findings and fixes, files changed, blockers, residual risks, and mark task/plan done.
