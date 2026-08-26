---
id: "05-docs-and-verification"
title: "Document and verify the feature"
status: done
wave: 5
depends_on: ["04-mobile-accessibility-and-tooling"]
plan: "plan.md"
spec: "../../specs/office/requirements/automations-pr-merged-trigger.md"
---

# Task 05: Document and Verify the Feature

## Intent

Make the shipped behavior discoverable and perform the proportional final audit without expanding the
feature scope.

## Acceptance

- Public automation docs explain configuration, linked-task scoping, poll latency, first-observation
  behavior, structurally bound archive target, and retry/manual-run limitations.
- The public feature-status matrix lists the merged-PR trigger accurately.
- Focused backend, frontend, desktop/mobile E2E, and docs validation commands pass.
- The final diff contains no unrelated compatibility or refactor changes.

## Documentation work

- Update `docs/public/automation-and-mcp.md` as the primary how-to/explanation page.
- Update `docs/public/feature-status.md` as the reference/status entry.
- Keep terminology consistent with the editor: "Pull request merged", linked task, repository filter,
  base branch, and up to one minute detection delay.

## Verification sequence

1. Run the focused backend package tests from Tasks 01-03.
2. Run the allowlist test and web typecheck.
3. Run desktop Chromium and mobile Chrome merged-PR automation E2E specs.
4. Run both public-doc validators.
5. Review `git diff --check`, changed-file scope, and task/plan/spec/ADR links. Record exact results.

## Files likely touched

- `docs/public/automation-and-mcp.md`
- `docs/public/feature-status.md`
- plan/task status fields and validation records

## Dependencies

Task 04 and all prior implementation tasks.

## Parallelism

`sequential` — this is the final integration and documentation checkpoint.

## Verification

- `cd apps/backend && go test ./internal/automation ./internal/backendapp ./internal/orchestrator ./internal/mcp/handlers ./internal/mcp/server`
- `cd apps && pnpm --filter @kandev/web test -- scripts/lib/guard-allowlist.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm e2e:run --project chromium tests/automations-pr-merged-trigger.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/mobile-automations-pr-merged-trigger.spec.ts`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`

## Risks

- Do not document manual Run as replay for a missed merge; it carries no event task id.
- Do not imply webhook immediacy or exactly-once delivery.
- If a focused test reveals a broader regression, update the spec and plan before widening the fix.

## Completed validation

- GREEN: `cd apps/backend && go test ./internal/automation ./internal/backendapp ./internal/orchestrator ./internal/mcp/handlers ./internal/mcp/server` (2,800 tests).
- GREEN: allowlist Vitest, web typecheck, mobile Chrome E2E (1 test), and Chromium merged-PR E2E (15 tests).
- GREEN: `node --test scripts/validate-public-docs.test.mjs` (58 tests).
- GREEN: `node scripts/validate-public-docs.mjs` (41 published pages validated).
- GREEN: final formatting and `git diff --check` review are part of the pre-commit handoff.
- Public docs now describe linked-task targeting, polling/first-observation semantics, archive binding,
  dedup/cap behavior, and manual-run/retry limitations.

## Output contract

Record every command and result, summarize remaining risks, update all plan/task statuses accurately,
and provide the final implementation handoff in the primary conversation.
