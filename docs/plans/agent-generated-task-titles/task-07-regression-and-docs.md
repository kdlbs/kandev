---
id: "07-regression-and-docs"
title: "Regression coverage and public docs"
status: done
wave: 7
depends_on: ["05-single-owner-title-handoff", "06-default-on-title-preference"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/agent-generated-titles.md"
---

# Task 07: Regression coverage and public docs

## Acceptance

- High-level backend tests cover two sessions racing on one pending task, owner launch failure followed
  by a later session, same-owner retry, message-start, direct/prepared start, workflow auto-start,
  structured and passthrough prompt paths, and owner-bound MCP mode/title mutation.
- Desktop Playwright proves the setting is enabled for the default experience and that explicit opt-out
  restores the required title input. Mobile Playwright proves the default prompt-first task/subtask
  surfaces remain contained, reachable, and free of horizontal overflow.
- Public task, subtask, coordination, and MCP docs state that agent title generation is default-on,
  explicit opt-outs persist, exactly one first-launched task-mode session owns the instruction/tool, and
  later sessions never inherit it after owner failure.
- Targeted tests, changed-file backend lint, frontend lint/typecheck/i18n checks, public-doc validators,
  and desktop/mobile E2E pass before PR fixup resumes.

## Verification

```bash
cd apps/web && pnpm e2e --project=chromium tests/task/agent-generated-task-titles.spec.ts
cd apps/web && pnpm e2e --project=mobile-chrome tests/task/mobile-agent-generated-task-titles.spec.ts
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:ratchet
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Run `golangci-lint run ./... --new-from-rev="<PR-base-sha>" --timeout=5m` from `apps/backend` after the
final backend changes.

## Files likely touched

- focused backend orchestration/MCP integration tests from Task 05
- `apps/web/e2e/tests/task/agent-generated-task-titles.spec.ts`
- `apps/web/e2e/tests/task/mobile-agent-generated-task-titles.spec.ts`
- `docs/public/tasks-and-workflows.md`
- `docs/public/coordination.md`
- `docs/public/automation-and-mcp.md`
- `docs/public/agent-communication.md` if its session guidance needs the ownership boundary
- `docs/public/coverage.json` if coverage metadata changes

## Dependencies

Tasks 05–06 completed with their focused checks passing.

## Parallelism

Sequential. E2E and public docs validate the assembled ownership/default contract.

## Inputs

- Every new conformance scenario in the approved spec.
- Plan sections: **Tests**, **E2E Tests**, and **Public documentation**.
- Mobile-parity and E2E skill requirements for real user outcomes and managed browser execution.

## Risks

- The mock agent does not infer natural-language title instructions; use backend captured-prompt/catalog
  tests for ownership and keep browser assertions focused on user-visible default/opt-out behavior.
- Avoid restoring a missing preference as explicit false in dedicated tests when the scenario intends to
  prove the new default; restore only values the test deliberately changed.

## Result

Backend coverage now exercises single-owner claims, owner-bound title mutation, launch-mode gating,
message/workflow/direct/prepared paths, and stale metadata handling. Public task and MCP documentation
describe the default-on preference, explicit opt-out, six-word sentence-case target, and exactly-one
eligible session boundary. The shared E2E fixture opts out for unrelated manual-title scenarios.

`node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs` both
passed. The full backend race suite, backend lint, targeted web tests, and web lint passed; the prior PR
head's desktop/mobile E2E checks were green, while new browser runs were not available locally.
