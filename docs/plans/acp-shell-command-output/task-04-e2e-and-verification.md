---
id: "04-e2e-and-verification"
title: "Command output E2E verification"
status: done
wave: 3
depends_on: ["02-acp-live-updates", "03-frontend-command-row"]
plan: "plan.md"
spec: "../../specs/ui/acp-shell-command-output.md"
---

# Task 04: Command Output E2E Verification

## Acceptance

- Desktop E2E proves persisted success, failure, and unknown-exit command rows expand to the expected output and exact exit labels.
- Mobile E2E proves wrapped/truncated output and exit status remain readable without overlap or page-level horizontal overflow.
- Targeted E2E, backend tests/lint, frontend tests/typecheck/lint, and formatting complete successfully, with failures reported rather than hidden.

## Verification

```bash
make -C apps/backend fmt
cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp
cd apps && pnpm --filter @kandev/web test -- components/task/chat/messages/tool-execute-message.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm e2e:run tests/chat/tool-execute-output.spec.ts tests/chat/mobile-tool-execute-output.spec.ts
make -C apps/backend test
make -C apps/backend lint
```

## Files likely touched

- `apps/web/e2e/tests/chat/tool-execute-output.spec.ts` (new)
- `apps/web/e2e/tests/chat/mobile-tool-execute-output.spec.ts` (new)
- `apps/web/e2e/pages/session-page.ts` only if a small reusable command-row helper is justified
- `docs/specs/ui/acp-shell-command-output.md` only if implementation reveals a contract correction
- `docs/plans/acp-shell-command-output/plan.md`

## Dependencies

Tasks 02 and 03.

## Inputs

- All spec scenarios.
- Existing `seedSessionMessage` pattern in `mobile-repeated-tool-activity.spec.ts`.
- Existing desktop/mobile Playwright projects and active-chat scoping in `SessionPage`.

## Output contract

Report exact commands and pass/fail results, desktop/mobile projects exercised, failure artifact paths, files changed, blockers, and residual risks. Set this task to `done`, update `plan.md`, and change the spec to `shipped` only when all acceptance criteria pass.
