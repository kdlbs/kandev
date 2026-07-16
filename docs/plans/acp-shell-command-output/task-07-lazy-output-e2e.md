---
id: "07-lazy-output-e2e"
title: "Lazy shell output E2E verification"
status: done
wave: 6
depends_on: ["06-frontend-output-disclosure"]
plan: "plan.md"
spec: "../../specs/ui/acp-shell-command-output.md"
---

# Task 07: Lazy Shell Output E2E Verification

## Acceptance

- Desktop E2E proves the full command is visible while output starts collapsed, no snapshot request occurs before expansion, and expanding a completed command fetches and renders its persisted transcript/result.
- Desktop E2E proves an expanded running command refreshes through a controlled sequence and stops requesting after terminal status; mobile E2E proves long commands and output stay within the viewport without overlap.
- Targeted E2E plus backend/frontend format, tests, typecheck, and lint pass, and the spec/plan statuses are updated only after all acceptance criteria are satisfied.

## Verification

```bash
cd apps/web && pnpm e2e:run --project chromium tests/chat/tool-execute-output.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-tool-execute-output.spec.ts
make -C apps/backend fmt
cd apps/backend && go test ./internal/task/models ./internal/task/handlers ./internal/task/service ./internal/backendapp
make -C apps/backend lint
cd apps && pnpm --filter @kandev/web test -- hooks/domains/session/use-shell-command-output.test.ts components/task/chat/messages/tool-execute-message.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
make fmt
make typecheck
make test
make lint
```

## Files likely touched

- `apps/web/e2e/tests/chat/tool-execute-output.spec.ts`
- `apps/web/e2e/tests/chat/mobile-tool-execute-output.spec.ts`
- `apps/web/e2e/pages/session-page.ts` only if a reusable active-chat disclosure helper removes duplication
- `docs/specs/ui/acp-shell-command-output.md` for status only, unless verification reveals a genuine contract correction
- `docs/specs/INDEX.md` for matching status
- `docs/plans/acp-shell-command-output/plan.md`
- `docs/plans/acp-shell-command-output/task-07-lazy-output-e2e.md`

## Dependencies

Tasks 05 and 06 must be integrated. This task changes no output storage or API behavior.

## Inputs

- All new lazy-output spec scenarios.
- Existing desktop/mobile shell-output E2E fixtures and `SessionPage.activeChat()` scoping.
- Task 05 handler contract and Task 06 test IDs/disclosure behavior.
- Invoke `/e2e`, `/mobile-parity`, `/qa`, and `/verify` for this integrated verification task.

## Output contract

Report network-request assertions, polling sequence/count, desktop/mobile projects, viewport checks, all verification commands and results, artifact paths for failures, files changed, blockers, and remaining risks. Set this task and plan to `done` and the spec/index to `shipped` only after the full acceptance and verification set passes.
