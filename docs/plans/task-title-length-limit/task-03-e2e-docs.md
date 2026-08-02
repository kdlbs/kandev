---
id: "03-e2e-docs"
title: "E2E coverage and public documentation"
status: done
wave: 2
depends_on: ["01-backend-mcp-contract", "02-frontend-title-limit"]
plan: "plan.md"
spec: "../../specs/tasks/title-length-limit.md"
---

# Task 03: E2E coverage and public documentation

## Acceptance

- Desktop Playwright proves an overlong remote PR prefill becomes a valid 60-character task title while retaining its remote source.
- Mobile Playwright proves overlong issue prefill and manual input share the cap without breaking dialog containment or causing horizontal overflow.
- Public task and MCP documentation state the 60-character contract and validation behavior.

## Verification

```bash
(
  cd apps/web
  pnpm e2e:run --host --project chromium tests/github/pr-action-create-task-dialog.spec.ts
)
(
  cd apps/web
  pnpm e2e:run --host --project mobile-chrome tests/task/mobile-create-task-remote-repo.spec.ts
)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/e2e/tests/github/pr-action-create-task-dialog.spec.ts`
- `apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts`
- `docs/public/tasks-and-workflows.md`
- `docs/public/automation-and-mcp.md`

## Dependencies

- Task 01: backend and MCP contract is complete.
- Task 02: frontend input and prefill behavior is complete.

## Parallelism

Sequential. This task verifies and documents the integrated behavior from Tasks 01 and 02.

## Inputs

- Spec scenarios for desktop/mobile input, remote prefill, and MCP validation.
- Plan sections: **E2E Tests**, **Public documentation**, and **Mobile design contract**.
- Existing Playwright fixtures and provider mocks in the two named spec files.

## Risks

- Run through `pnpm e2e:run` so the production Vite bundle and Go backend are rebuilt; `--no-build` would risk testing stale artifacts.
- Preserve the existing mobile geometry assertions and use the configured `mobile-chrome` project rather than a per-test device override.

## Output contract

Report changed files, exact E2E/docs commands and results, rendered mobile evidence, blockers and remaining risks, then mark this task `done` and update its checkbox in `plan.md`.
