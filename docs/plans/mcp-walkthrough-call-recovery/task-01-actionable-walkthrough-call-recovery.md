---
id: "mcp-walkthrough-call-recovery-01"
title: "Make walkthrough call recovery actionable"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/mcp-tool-argument-validation.md"
---

# Task 01: Make Walkthrough Call Recovery Actionable

## Acceptance

1. A `show_walkthrough_kandev` call whose step omits `text` fails before
   backend dispatch and names `/steps/0`, the `required` constraint, and the
   missing `text` schema property without exposing any submitted value.
2. First-turn task context names show/get/delete walkthrough tools, and both it
   and the built-in walkthrough request state that every `steps` item requires
   `file`, `line`, and `text`, with tests derived from the registered schema.
3. Public MCP reference documents missing-property diagnostics; all focused Go
   and public-doc checks pass.
4. Existing installations refresh untouched historical walkthrough prompts in
   their loader-normalized stored form to the corrected embedded content
   without overwriting unrecognized content, user edits, or user-owned prompt
   conflicts.

## Verification

Follow strict TDD, then run:

```bash
cd apps/backend
go test ./internal/prompts/store ./internal/mcp/server ./internal/sysprompt
cd ../..
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/mcp/server/tool_argument_validation.go`
- `apps/backend/internal/mcp/server/tool_argument_validation_test.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
- `apps/backend/internal/prompts/store/sqlite.go`
- `apps/backend/internal/prompts/store/sqlite_test.go`
- `apps/backend/internal/prompts/store/testdata/`
- `apps/backend/config/prompts/kandev-context.md`
- `apps/backend/config/prompts/changes-walkthrough.md`
- `docs/public/automation-and-mcp.md`
- `docs/specs/integrations/requirements/mcp-tool-argument-validation.md`
- `docs/specs/INDEX.md`
- `docs/plans/mcp-walkthrough-call-recovery/plan.md`
- `docs/plans/mcp-walkthrough-call-recovery/task-01-actionable-walkthrough-call-recovery.md`

## Dependencies

None.

## Parallelism

`sequential` — validator behavior, schema-derived prompt coverage, and contract
documentation form one regression repair.

## Inputs

- [MCP Tool Argument Validation spec](../../specs/integrations/requirements/mcp-tool-argument-validation.md)
- [Fix plan](plan.md)
- `docs/decisions/2026-08-01-validate-mcp-tool-arguments.md`
- `sanitizedToolArgumentError`, `firstKeywordFailure`, and
  `buildWalkthroughStepSchemaItem`
- `TestAskUserQuestionDocs_MatchSchema` as schema-to-prompt consistency pattern
- Built-in `changes-walkthrough` prompt and task-mode Kandev context

## Risks

- Do not expose rejected argument values through a generic validator message.
- Preserve error formatting for every non-`required` validation keyword.
- Do not change nested-map openness or add client-specific retries.

## Output contract

Report red/green test evidence, exact error shape, prompt and public-doc files,
all command results, residual risks, and synchronized task/plan/spec statuses.

## Results

Completed on 2026-08-03.

- RED: focused server tests failed on the old behavior because the validation
  error did not name missing `text`, task context did not advertise the
  walkthrough tools, and the embedded prompts did not state the required item
  shape.
- GREEN: required failures now retain the object path and keyword while adding
  quoted schema property names, for example
  `validation failed at /steps/0 (keyword: required; missing: "text")`.
  Submitted argument values remain absent and invalid calls still do not reach
  backend dispatch.
- Task context now lists show/get/delete walkthrough tools. It and the built-in
  walkthrough request both describe `steps` as an ordered array whose items
  require `file`, `line`, and `text`; a schema-derived regression test guards
  this contract.
- Public MCP reference now documents the actionable, redacted missing-property
  behavior.
- `cd apps/backend && go test ./internal/mcp/server ./internal/sysprompt` —
  passed.
- `node --test scripts/validate-public-docs.test.mjs` — passed, 58 tests.
- `node scripts/validate-public-docs.mjs` — passed, 41 published pages.
- `git diff --check` — passed.
- Browser E2E was intentionally not run because no rendered UI behavior
  changed.
- PR review remediation refreshes the exact loader-normalized stored content of
  the two untouched historical prompt revisions, preserves unrecognized
  content, edited built-ins, and user-owned conflicts, sorts missing schema
  properties deterministically, asserts top-level `steps` requiredness, and
  registers backend cleanup with `t.Cleanup`.
- Remediation RED tests reproduced both stale stored revisions and unsorted
  output; the focused GREEN regressions passed afterward.
- `cd apps/backend && go test ./internal/prompts/store ./internal/mcp/server ./internal/sysprompt` — passed.
- `cd apps/backend && golangci-lint run ./... --new-from-rev="0ca00c2aca4430a5e4f06874ba960743de1acf9a" --timeout=5m` — passed with 0 issues.
- Follow-up review RED coverage proved the fixture-file hashes did not recognize
  historical content in the exact trimmed form stored by `promptcfg.Get`.
- Follow-up GREEN coverage uses loader-normalized historical hashes and proves
  equal-timestamp unrecognized content remains unchanged.
- The affected Go packages, changed-code lint, 58 public-doc tests, 41-page
  public-doc validation, and `git diff --check` all passed again.
