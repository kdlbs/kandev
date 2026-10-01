---
id: "01-classify-claude-session-limit"
title: "Classify Claude session limits and parse reset clocks"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CLAUDE-SESSION-LIMIT-001
acceptance_criteria:
  - AC-AGENTS-CLAUDE-SESSION-LIMIT-001.1
  - AC-AGENTS-CLAUDE-SESSION-LIMIT-001.2
  - AC-AGENTS-CLAUDE-SESSION-LIMIT-001.3
  - AC-AGENTS-CLAUDE-SESSION-LIMIT-001.4
  - AC-AGENTS-CLAUDE-SESSION-LIMIT-001.5
  - AC-AGENTS-CLAUDE-SESSION-LIMIT-001.6
  - AC-AGENTS-CLAUDE-SESSION-LIMIT-001.7
system_design:
  - ../../specs/agents/system-design/claude-session-limit-classification.md
---

# Task 01: Classify Claude Session Limits and Parse Reset Clocks

## Summary

Recognize the observed Claude notice as a hard quota failure and parse its explicitly zoned reset clock.
Prove that terminal projection and existing recovery consumers receive the result without new provider branches.

## In scope

- Add the provider-scoped session signature and clock-only reset branch described by the system design.
- Add failing classifier and parser tests before production changes.
- Add ACP, Kanban, and dynamic-engine contract tests named in the plan.
- Preserve structured metadata precedence, existing dated formats, and diagnostic redaction.

## Out of scope

- Fixed-profile automatic resume, workflow deferral, rendered recovery changes, and saved-policy changes.
- Unobserved Claude period signatures or changes to other provider rules.
- Production changes outside `routingerr` unless new evidence requires revising this package first.

## Acceptance

1. The exact issue notice classifies as high-confidence hard quota and carries the correctly zoned hint through existing terminal projection.
2. Clock tests cover same day, next day, equality, midnight, noon, omitted minutes, backend-zone independence, and daylight-saving boundaries.
   Invalid clocks, missing zones, unknown names, `Local`, ambiguous clocks, and nonexistent clocks produce no hint.
3. Structured precedence and other providers retain their behavior.
   Fixed Kanban quota recovery remains manual, while dynamic hard policy excludes shared-binding siblings and honors the reset deadline.

## TDD sequence

1. Mark this work order `in_progress` and synchronize the plan.
2. Add `TestClassifyClaudeSessionLimit` and `TestParseClaudeResetClock` with the exact observed notice.
3. Run these tests and record failures for the unknown classification and nil hint.
4. Add the smallest rule and parser correction, then add the remaining plan regressions.
5. Run every verification command and record the actual results.
6. Mark this work order `done` and synchronize the plan only after all checks pass.

Classifier cases include straight and curly apostrophes, `you have`, case and whitespace variants, and both prompt-send and streaming phases.
Negative cases include a different provider, bare `session limit`, approaching limits, suffix text such as `limitless`, and rate-limit wording.

Parser cases use fixed `now` values and compare absolute instants.
Use Helsinki before and after 11:10am, exact equality, UTC, and a complete nested IANA name at the raw parser boundary.
Keep the existing Codex dated, leap-date, offset, and year-rollover cases.
Reject invalid hours, minutes, missing meridiem, missing parentheses, unknown zones, abbreviations, and host-local zones.
Use Helsinki's spring gap as a no-hint case while that nonexistent clock is still upcoming.
If the current day's gap clock has elapsed, assert rollover to the next valid day's clock.
Keep the autumn repeated 03:30 clock as a no-hint case while that ambiguous clock is upcoming.
Use valid clocks across each transition to prove calendar rollover and offset changes.

The dynamic regression uses two Claude candidates sharing a binding and one healthy candidate on another binding.
Apply the classified notice through the default hard policy and assert the distinct binding is selected.
Use `WithClock` to place the engine before the parsed reset and assert the shared circuit is open before that deadline.
At the reset instant, assert one shared-binding candidate acquires the exclusive probe while the sibling remains excluded.
Release a successful probe and assert the circuit closes; the reset must be later than the engine's minimum backoff.
Keep existing output, tool-effect, and generation-fence tests passing.

## Verification

Run this complete block from the repository root:

```bash
(cd apps/backend && go test ./internal/agent/runtime/routingerr -run 'TestClassifyClaudeSessionLimit$|TestParseClaudeResetClock$' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/routingerr ./internal/agent/runtime/dynamic -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestProviderErrorFromErrorClaudeSessionLimit$' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestClassifyKanbanFailureClaudeSessionLimit$|^TestClaudeQuotaFixedProfileRemainsManual$' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/claude-session-limit-classification
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/agents/requirements/claude-session-limit-classification.md',
  'docs/specs/agents/system-design/claude-session-limit-classification.md',
  'docs/plans/claude-session-limit-classification/plan.md',
  'docs/plans/claude-session-limit-classification/task-01-classify-claude-session-limit.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/backend/internal/agent/runtime/routingerr/rules.go', status: 'modified' },
    ...paths.map(filename => ({ filename, status: 'modified' })),
  ],
  fileContents: Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify({ status: result.status, errors: result.errors }, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

Run the first command before implementation for red evidence, then repeat it after the correction.
Use `gofmt` on changed Go files before final verification.
The Node preflight validates the complete package against a representative runtime change without publishing a GitHub status.

## Files likely touched

- `apps/backend/internal/agent/runtime/routingerr/rules.go`
- `apps/backend/internal/agent/runtime/routingerr/resethint.go`
- `apps/backend/internal/agent/runtime/routingerr/classify_test.go`
- `apps/backend/internal/agent/runtime/routingerr/resethint_test.go`
- `apps/backend/internal/agent/runtime/dynamic/engine_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr_test.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_claude_test.go` (new test file)
- This work order and `plan.md` for execution results.

## Dependencies

None. Existing Codex reset parsing, ACP projection, and recovery policies are already present in the investigation checkout.
Re-read these boundaries if the base changes before implementation.

## Risks

- Embedded timezone data changes binary size.
- Daylight-saving normalization must not silently choose a nonexistent or repeated clock.
- Upstream sanitization can remove nested zone components. Keep redaction intact and return no hint for that projected shape.
- Recovery flags do not authorize fixed-profile replay or unsafe dynamic fallback.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/agents/requirements/claude-session-limit-classification.md).
- [System design](../../specs/agents/system-design/claude-session-limit-classification.md).
- [Plan evidence and test mapping](plan.md).
- Existing `routingerr/resethint_test.go`, `dynamic/engine_test.go`, and ACP projection tests.
- Existing provider-neutral recovery and provider-class ADRs linked by the system design.

## Results

Done. The observed Claude session-limit signature now classifies as a high-confidence hard quota failure, and clock-only reset hints resolve in the supplied UTC or IANA zone. The parser rejects invalid, unsupported, nonexistent, and ambiguous clocks without changing the quota classification. ACP projection, fixed-profile Kanban policy, dynamic shared-binding routing, and reset-time probe behavior are covered by regressions.

The initial classifier/parser tests failed on the original unknown classification and nil reset hint, then passed after implementation. The targeted `routingerr` and `dynamic` packages, ACP projection regression, Kanban classification and manual-policy regressions, backend build, documentation validation, specification lint, PR-documentation coverage preflight, and whitespace check all passed. The backend build emitted only the expected unsigned macOS agentctl artifact warnings because no signing tool was installed.

Review correction: the original DST loop stopped when today's selected clock fell in a spring gap, even if that wall-clock time had already elapsed. Added a fixed-clock regression for 2026-03-29 12:00 UTC and now advance to the next calendar day only in that elapsed-gap case. The upcoming spring-gap and autumn-ambiguity no-hint cases remain intact. `go test ./internal/agent/runtime/routingerr -run 'TestParseClaudeResetClock' -count=1` and `go test ./internal/agent/runtime/routingerr ./internal/agent/runtime/dynamic -count=1` passed after the correction.
