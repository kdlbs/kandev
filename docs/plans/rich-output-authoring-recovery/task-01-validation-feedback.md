---
id: "01-validation-feedback"
title: "Relevant schema validation feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.2
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.12
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.13
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---
# Task 01: Relevant schema validation feedback

## Summary

Project the authoritative schema failures into useful, sanitized feedback.
Recognized component/source forms must not inherit errors from incompatible
alternatives. Prove rejection followed by correction through the actual wrapper.

## In scope

- Start with the plan's malformed Table input in
  `TestToolArgumentValidationSelectsRelevantUnion`. Assert the error names
  `/spec/elements/services/props/columns/0`, expected object and required
  `key`/`label`; it must not mark rows/title/columns unknown or demand chart,
  StatusList, Callout, Stack or CSV-only fields.
- Cover CSV Table, inline/CSV Chart, Stack child bounds, malformed StatusList
  items, missing/unknown type, both/neither sources, independent siblings and
  root unknown fields, nested type/enum constraints, multiple matched `oneOf`
  branches and generic non-discriminated alternatives.
- Use compiled schema/instance context to prune incompatible alternative
  causes only in the diagnostic projection. Keep other failures and unchanged
  strict validation, portable roots, stale v1 errors and task-scope rules.
- Add `TestRichOutputRejectedCallRecovery` for task and Office wrapper calls:
  no backend action on malformed CSV calls; corrected inline accepted;
  corrected CSV uses only its authorized source and returns the normal snapshot.
  Include private-value sentinels and eight-entry/4-KiB detail bound checks.

## Out of scope

Schema relaxation, argument repair, rich-output-only validation engine, new
tooling/dependencies, client transport changes, renderer changes and model runs.

## Acceptance

1. The registered-validator regression fails before implementation for the
   reproduced contradictory error and passes after the minimum correction.
2. All union cases receive truthful relevant diagnostics or labelled unresolved
   alternatives; non-union compatibility and secret-value exclusion pass.
3. Wrapped rejected/corrected calls preserve no-read-on-error, task/Office
   visibility, normal results and the closed catalog.

## Verification

Run from repository root; use a private Go cache if the shared cache is unstable.

```bash
(cd apps/backend && go test ./internal/mcp/server -run 'TestToolArgumentValidationSelectsRelevantUnion|TestRichOutputRejectedCallRecovery' -count=1)
(cd apps/backend && go test ./internal/mcp/server -count=1)
(cd apps/backend && golangci-lint run ./internal/mcp/server/...)
git diff --check
```

Record the first command's expected RED result before production edits, then
its GREEN result and every remaining command. Do not infer completion from a
duplicate run while an earlier command still has a live session handle.

## Files likely touched

- `apps/backend/internal/mcp/server/tool_argument_validation.go`
- New narrowly scoped diagnostic helper under that package if limits require it.
- `apps/backend/internal/mcp/server/tool_argument_validation_test.go`
- `apps/backend/internal/mcp/server/rich_output_test.go` or a dedicated wrapped-call recovery test file.

## Dependencies

None. Reuse the completed json-render/catalog baseline in the workspace.

## Risks

Schema traversal must preserve simultaneous failures without interpreting all
union alternatives as simultaneous obligations. Use exported dependency types;
do not invoke raw validator formatting that can expose instance values.

## Parallelism

`sequential`

## Inputs

- [Owning requirements](../../specs/agents/requirements/agent-rich-output.md), criteria 003.12 and 003.13.
- [System design](../../specs/agents/system-design/agent-rich-output.md), Evolution and authoring feedback.
- [Plan](plan.md), reproduction and diagnostic projection boundaries.
- Existing sanitizer, session-bound-task and rich-output source tests.

## Results

Completed 2026-10-12. RED reproduced the incorrect union message in the real
registered validator (retained privately as `recovery-red.log`). GREEN covers
12 component/source cases, generic alternatives/overlap/allOf, independent
failures, bounded sanitized details and task/Office rejected/corrected calls.

- Focused regression command: passed, including `TestToolArgumentUnion` cases.
- `go test ./internal/mcp/server -count=1`: passed (6.979s final run).
- `golangci-lint run ./internal/mcp/server/...`: 0 issues after extracting schema-index traversal helpers and correcting append loops.
- `git diff --check`: passed.

Go commands used `GOFLAGS='-trimpath -p=4'`, private `GOCACHE` and
`GOLANGCI_LINT_CACHE` under the privately retained isolated environment. Full authoritative
validation still runs before source reads; only its diagnostic projection
selects relevant branches. No argument repair or new dependencies.

PR review follow-up: the four-data-component error lists Chart, Metrics,
FilePreview, Table and StatusList. The shared five-StatusList regression now
checks all counted type names through the wrapped tool. It failed before the
message correction and passed with the full MCP/server and sysprompt suites.
