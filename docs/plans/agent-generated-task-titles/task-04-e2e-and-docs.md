---
id: "04-e2e-and-docs"
title: "End-to-end coverage and public docs"
status: done
wave: 4
depends_on: ["01-backend-title-lifecycle", "02-mcp-title-tool", "03-frontend-title-flow"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/agent-generated-titles.md"
---

# Task 04: End-to-end coverage and public docs

> Continuation note: Task 07 updates coverage and documentation for the default-on, single-owner
> contract. This completed task records the original opt-in behavior.

## Acceptance

- Desktop E2E proves saved preference persistence, absent title inputs, empty-prompt gating, and the
  immediate six-word provisional title. Backend service/MCP tests cover the agent replacement and
  idempotent late-call behavior.
- Mobile E2E proves the subtask creation surface remains usable, contained, and free of document-level
  horizontal overflow with the prompt as the first editable field; the shared task dialog uses the
  same preference/payload contract covered by focused frontend tests.
- Public task, subtask, and MCP documentation describes the setting, fallback/pending lifecycle, mode
  boundary, pending-only tool availability/token behavior, six-word sentence-case target, and
  short-title guidance; public-doc validators pass.

## Verification

```bash
cd apps/web && pnpm e2e --project=chromium tests/task/agent-generated-task-titles.spec.ts
cd apps/web && pnpm e2e --project=mobile-chrome tests/task/mobile-agent-generated-task-titles.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

The desktop and mobile scenarios each passed. The two `node` commands run from the repository root;
the public-doc suite passed all 58 tests and validated 41 published pages.

## Files likely touched

- `apps/web/e2e/tests/task/agent-generated-task-titles.spec.ts`
- `apps/web/e2e/tests/task/mobile-agent-generated-task-titles.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- relevant task/subtask page objects if a stable helper is missing
- `docs/public/tasks-and-workflows.md`
- `docs/public/coordination.md`
- `docs/public/automation-and-mcp.md`
- `docs/public/agent-communication.md`
- `docs/public/coverage.json`

## Dependencies

Tasks 01–03 completed with their targeted checks passing.

## Parallelism

Sequential. E2E depends on the assembled backend/frontend flow; docs must describe the final verified
contract.

## Inputs

- Every conformance scenario in the spec.
- Plan sections: **E2E Tests** and **Public documentation**.
- E2E rules: managed headless runner, production rebuild, API seeding with UI assertions, and restored
  user settings after each test.

## Risks

- The mock agent does not infer natural-language instructions; use an explicit mock MCP script while
  backend prompt-capture tests prove the instruction wording and ordering.
- Capture the provisional state deterministically (for example create-only before launch) rather than
  racing an auto-started mock agent.
- `mobile-*.spec.ts` must use the configured `mobile-chrome` project and assert real user outcomes.

## Output contract

Report scenarios covered, docs changed, the exact E2E/docs commands and results, blockers or risks, and
update this task plus `plan.md` status in the same conversation.
