---
id: "02-guidance-and-docs"
title: "Fragment guidance and public documentation"
status: done
wave: 2
depends_on:
  - "01-bounded-reads"
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-READ-003
acceptance_criteria:
  - AC-TASKS-PLAN-READ-003.1
  - AC-TASKS-PLAN-READ-003.2
  - AC-TASKS-PLAN-READ-003.3
system_design:
  - ../../specs/tasks/system-design/plan-partial-reads.md
---

# Task 02: Fragment guidance and public documentation

## Summary

Make the bounded read and existing partial writes the advertised path for
focused plan changes. Keep full-context planning available and preserve the
session and workflow instructions around the plan guidance.

## In scope

- Update task/Office context, plan-mode/default-plan instructions, applicable
  built-in workflow text, and the active-plan `buildDocumentContext` block.
- Explain character offsets, first-page/continuation calls, version reuse,
  post-write offset invalidation, and exact-edit/append selection. Never imply
  that a partial read is a complete replacement document.
- Extend sysprompt and document-context tests to assert new guidance, preserve
  system wrappers and identity/phase/question gates, and keep excluded profiles
  free of plan authority. Use behavioral instructions rather than snapshots of
  whole prompts. Preserve the existing agent-only i18n exemption.
- Update existing MCP public reference/how-to sections with the final supported
  fields, example calls, bounds, EOF, conflicts, and full-read compatibility.
- Reconcile any contradictory full-read-only wording in the existing safe-edit
  design when the new read implementation is ready. Keep existing safety
  requirements and recovery rules intact.
- Record results and promote the paired draft specs and plan after both work
  orders pass and implementation matches the package.

## Out of scope

- New read/write implementation, changes to approval barriers, or model routing.
- Rendered UI copy, layout, touch interactions, and viewport behavior.

## Acceptance

1. All applicable agent plan guidance advertises bounded reads and fragment
   writes without forcing an unnecessary full read or weakening safety gates.
2. Prompt regression tests prove the active-plan and system instruction paths
   preserve their wrappers and workflow behavior; public examples match the
   implementation and pass documentation validation.
3. Required checks are recorded, paired specs reflect delivered behavior, and
   work-order/plan lifecycle is updated only after both orders pass.

## Verification

If workspace dependencies are absent, first run
`(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -trimpath ./internal/sysprompt -count=1)
(cd apps/web && pnpm exec vitest run hooks/use-message-handler.test.ts hooks/use-message-handler.plan-comments.test.ts)
(cd apps/web && pnpm exec eslint hooks/use-message-handler.ts)
(cd apps/web && pnpm run i18n:ratchet)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the repository's exported `.github/scripts/pr-docs.cjs` `validateCoverage`
preflight with changed work orders and their linked plan/requirement/design
contents; record accepted cross-references. No new browser/mobile E2E is needed:
the only frontend change builds the same agent prompt for all viewports and
has no rendered or viewport-dependent behavior.

## Files likely touched

- `apps/backend/config/prompts/kandev-context.md`
- `apps/backend/config/prompts/office-context.md`
- `apps/backend/config/prompts/plan-mode.md`
- `apps/backend/config/prompts/default-plan-prefix.md`
- `apps/backend/config/workflows/plan-and-build.yml`
- `apps/backend/config/workflows/feature-dev.yml`
- `apps/backend/internal/sysprompt/sysprompt_test.go`
- `apps/backend/internal/sysprompt/plan_fragment_guidance_test.go` (new)
- `apps/web/hooks/use-message-handler.ts`
- `apps/web/hooks/use-message-handler.test.ts`
- `docs/public/automation-and-mcp.md`
- `docs/public/tasks-and-workflows.md`
- `docs/public/agent-communication.md`
- `docs/public/websocket-api.md`
- `docs/specs/tasks/system-design/plan-safe-edits.md` (contract reconciliation)
- `docs/specs/tasks/requirements/plan-partial-reads.md` (lifecycle)
- `docs/specs/tasks/system-design/plan-partial-reads.md` (lifecycle)
- `docs/plans/plan-partial-reads/plan.md` and this work order (results/lifecycle)

## Dependencies

Task 01 must pass before advertising range arguments.

## Risks

- Prompt changes must not replace or weaken hard workflow rules.
- A current version from a partial read protects a write but does not establish
  that the agent understands the whole plan; full-context tasks still need it.
- Cached schemas must refresh; guidance must respect discovered capabilities.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/plan-partial-reads.md), `003`.
- [Design](../../specs/tasks/system-design/plan-partial-reads.md), read/edit flow
  and guidance/public documentation.
- Existing `buildDocumentContext`, sysprompt tests, and public plan-write guides.
- Backend/web `AGENTS.md`, `/docs-maintainer`, and mobile-parity assessment.

## Results

Completed in the primary session after Task 01 passed.

- Guidance regression tests were added before prompt implementation and failed
  against the old guidance. The new fragment instructions passed those tests.
- The existing context-size guard initially caught excessive prompt growth.
  Compact wording now keeps the task template at 2,699 bytes, below the unchanged
  2,800-byte guard, and preserves the workflow/delegation/marker rules.
- The complete `internal/sysprompt` suite passed with `-trimpath`.
- Both document-context Vitest files passed: 47 tests across two files.
- Prettier, ESLint for the changed hook and test, and `i18n:ratchet` passed.
  The existing agent-only prompt exemption remains applicable.
- Public-doc validator tests passed (62 tests); validation passed for 47 pages.
- Catalog validation, all-spec lint, and 36 spec-linter tests passed.
- The complete affected MCP server, handler, plan WebSocket bridge, and contract
  package suites passed with `-trimpath`.
- The paired specs and implementation plan were promoted to their delivered
  states. Actual changed-file documentation coverage passed for both work
  orders and their linked requirements, design, and plan. Commit hooks run
  during delivery.

No rendered UI or viewport-dependent behavior changed. The shared prompt-builder
unit coverage exercises the frontend change; no browser/mobile E2E was added.
