---
id: "01-status-discovery"
title: "Clarify StatusList discovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.6
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.10
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.11
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.13
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.14
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
  - ../../specs/agents/system-design/agent-rich-output-failures.md
---

# Task 01: Clarify StatusList discovery

## Summary

Expose the StatusList enum in the short tool description using its schema as
the source. Preserve strict validation and the existing valid passed example.

## In scope

- Add failing tests for every advertised status and mutation of the source enum.
- Extend generated item-field guidance narrowly; compress repeated policy text
  to retain the existing 650-rune budget and all routing/shape guarantees.
- Regenerate checked artifacts instead of editing generated guidance directly.
- Verify the MCP registration exposes the corrected description in task and
  Office modes. Verify passed succeeds and success remains rejected.

## Out of scope

UI, schema widening, status aliases, new instruction budgets and provider changes.

## Acceptance

- Generated and registered descriptions list all five schema-defined statuses,
  and generation follows a mutated enum without a separately maintained list.
- The valid StatusList example uses passed and remains accepted. The unknown
  success status still fails before workspace reads or partial output.
- Existing description budget, discovery routing and generated drift checks pass.

## Verification

Run from the repository root. Install dependencies once if this worktree lacks them.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/rich-output/guidance.test.ts scripts/generate-rich-output-catalog.test.ts lib/rich-output/conformance.test.ts)
(cd apps/web && pnpm run rich-output:generate)
(cd apps/web && pnpm run rich-output:check)
(cd apps/backend && go test -trimpath ./internal/mcp/server -run 'TestRichOutput' -count=1)
(cd apps/web && pnpm exec eslint lib/rich-output/guidance.ts lib/rich-output/guidance.test.ts scripts/generate-rich-output-catalog.test.ts)
bash scripts/lint-go-changed apps/backend/internal/mcp/server/rich_output_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the new description test red before editing production logic, then all listed
checks green. If another test file is added, include it in the recorded commands.

## Files likely touched

- `apps/web/lib/rich-output/guidance.ts`
- `apps/web/lib/rich-output/guidance.test.ts`
- `apps/web/scripts/generate-rich-output-catalog.test.ts`
- `apps/web/lib/rich-output/examples.ts` (only if a valid example needs adjustment)
- `apps/backend/internal/mcp/server/rich_output_catalog_v1.guidance.txt` (generated)
- `apps/backend/internal/mcp/server/rich_output_test.go`
- Generated schema only if its checked example changes.

## Dependencies

None.

## Risks

Avoid rewriting the generator into an unbounded recursive schema printer.
A better description reduces ambiguity but cannot guarantee agent compliance.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/agents/requirements/agent-rich-output.md), criterion 003.14.
- [Failure design](../../specs/agents/system-design/agent-rich-output-failures.md), Discovery.
- Existing guidance mutation/overflow tests and backend description tests.

## Results

Implemented the schema-derived `StatusList` enum in generated guidance and
verified that task and Office MCP registrations advertise all five values.
The valid `passed` example remains accepted, and `success` remains rejected.
The generated guidance stays within its existing 650-rune limit.

Verification passed:

- Targeted web tests: `guidance.test.ts`, `generate-rich-output-catalog.test.ts`,
  and `conformance.test.ts`.
- `pnpm run rich-output:generate` and `pnpm run rich-output:check`.
- Backend `TestRichOutput` tests.
- Targeted ESLint and `bash scripts/lint-go-changed
  apps/backend/internal/mcp/server/rich_output_test.go` (`0 issues`).
- `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.
