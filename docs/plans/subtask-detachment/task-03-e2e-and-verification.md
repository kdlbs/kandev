---
id: "03-e2e-and-verification"
title: "E2E and verification"
status: done
wave: 2
depends_on: ["01-backend-detachment", "02-frontend-actions"]
plan: "plan.md"
spec: "../../specs/tasks/subtask-detachment.md"
---

# Task 03: E2E and verification

## Acceptance

- Desktop E2E proves sidebar and three-dot detachment, live root promotion, root-menu absence, workspace warning, and descendant preservation.
- Mobile E2E proves the touch-accessible action and confirmation produce the same outcome.
- Focused checks and the repository format, typecheck, test, and lint pipeline pass.

## Verification

```bash
cd apps/web && rtk pnpm e2e:run --host e2e/tests/task/subtask-detachment.spec.ts
cd apps/web && rtk pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-subtask-detachment.spec.ts
rtk make fmt
rtk make typecheck
rtk make test
rtk make lint
```

All acceptance checks passed. The desktop project covers sidebar and card actions, and the mobile project covers the touch action sheet. Repository formatting, typecheck, tests, and lint pass; E2E runs require local listener access outside the filesystem sandbox.

## Files likely touched

- `apps/web/e2e/tests/task/subtask-detachment.spec.ts`
- `apps/web/e2e/tests/task/mobile-subtask-detachment.spec.ts`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/pages/kanban-page.ts`
- Any focused regression test files identified during QA.

## Dependencies

- Task 01: Backend detach contract.
- Task 02: Frontend detach actions.

## Inputs

- All spec scenarios.
- Existing sidebar context-menu and kanban action-menu E2E patterns.
- `.agents/skills/e2e/SKILL.md`, `.agents/skills/mobile-parity/SKILL.md`, and `.agents/skills/verify/SKILL.md`.

## Output contract

Report E2E evidence, complete verification commands and outcomes, files changed, blockers, remaining risk, and update this task plus `plan.md` to `done` when acceptance passes.
