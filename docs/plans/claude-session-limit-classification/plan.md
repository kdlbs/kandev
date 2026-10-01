---
created: 2026-10-01
status: done
requirements:
  - REQ-AGENTS-CLAUDE-SESSION-LIMIT-001
system_design:
  - ../../specs/agents/system-design/claude-session-limit-classification.md
legacy_specs: []
---

# Implementation Plan: Claude Session-Limit Classification

## Overview

Repair [issue #4131](https://github.com/kdlbs/kandev/issues/4131) through one sequential work order.
Add the missing provider signature and reset-clock format, then prove that existing consumers receive the result.
This package is executing as one sequential work order.

## Evidence and root cause

Investigation used checkout `d2adc37ffebee6893275c4a47bcd91a2033a8271`.
The canonical issue has no image attachments or comments.
Its complete error text is:

```text
Internal error: You've hit your session limit · resets 11:10am (Europe/Helsinki)
```

`rules.go` recognizes Claude credit exhaustion and rate-limit wording but has no session-limit rule.
`resethint.go` accepts dated `try again at` clocks with UTC labels or numeric offsets.
It accepts neither the clock-only `resets` form nor its parenthesized IANA zone.

A temporary package-local test reproduced both gaps and was removed after execution.
The observed result was `agent_runtime_error`, `low`, `phase.poststart.unknown`, with retry and fallback false and a nil hint.
At fixed time `2026-10-01T08:03:00Z`, the clock parser also returned nil instead of `2026-10-01T08:10:00Z`.
The reproduction command was:

```bash
(cd apps/backend && go test ./internal/agent/runtime/routingerr -run TestIssue4131TemporaryEvidence -v -count=1)
```

This is evidence of current behavior, not a completed implementation check.
The temporary test is absent from the final diff.

## Scope

### In scope

- Claude's observed session-limit notice, its spelling variants, and bounded negative cases.
- Explicitly zoned reset clocks, calendar rollover, validation, and timezone-data availability.
- Regression coverage through terminal projection, Kanban classification, and dynamic routing.
- Existing structured precedence and Codex date-parser compatibility.

### Out of scope

- Automatic fixed-profile resume or workflow deferral, tracked separately by [#3811](https://github.com/kdlbs/kandev/issues/3811).
- New recovery UI, changes to saved policies, or relaxed effect-safety gates.
- Generic Claude period matching or changes to other provider rules.
- Broad sanitizer changes or recovery of redacted timezone components.

## Technical approach

The [system design](../../specs/agents/system-design/claude-session-limit-classification.md) owns parsing and consumer contracts.
Add `claude.stderr.session_limit.v1` in `apps/backend/internal/agent/runtime/routingerr/rules.go`.
Add a clock-only branch in `resethint.go`, with `time/tzdata` available as the standard fallback.
Reuse `parseClockHour`, numeric validation, and calendar validation where their contracts fit.
Keep structured reset precedence in `routingerr.Classify` unchanged.

No production changes are expected outside `routingerr`.
Integration tests must pass the observed terminal error through `ProviderErrorFromError` and `classifyKanbanFailure`.
Dynamic tests must pass the classified quota error through `Engine.ApplyFailure`.
Use the current test seams, including `WithClock` and the credential circuit store.
The circuit remains open after its deadline until the existing exclusive probe succeeds; the regression verifies probe eligibility at reset and sibling exclusion while that probe is held.

| Provider and transport | Identity and evidence | Intended behavior | Evidence | Unsupported fallback |
| --- | --- | --- | --- | --- |
| Claude ACP terminal error | `claude-acp`, complete issue message | Hard quota with zoned reset | Projection and classifier tests | Invalid clock keeps quota without hint |
| Claude dynamic route | Current effect-safe failure, shared binding | Existing hard policy and binding circuit | Dynamic engine regression | Existing manual gate for unsafe attempts |
| Claude fixed Kanban profile | Terminal error with agent identity | Quota classification, manual recovery | Kanban classification and policy regression | Existing manual recovery |
| Codex ACP | Existing dated notice | Existing parser and rule behavior | Existing routingerr tests | Unzoned clock produces no hint |
| Other ACP provider | Same Claude prose | Existing provider rules | Negative classifier cases | Unknown unless an existing rule matches |

## Regression coverage

The issue-level classifier and reset-clock tests were run first and failed on the original unknown classification and missing reset hint.
The remaining regressions cover the bounded variants, unsupported inputs, compatibility boundaries, and existing recovery consumers.

| Acceptance criteria | File and proposed regression |
| --- | --- |
| `.1`, `.7` | `routingerr/classify_test.go`: `TestClassifyClaudeSessionLimit`, `TestClassifyClaudeSessionLimitRejectsUnrelatedText` |
| `.2`, `.3`, `.4` | `routingerr/resethint_test.go`: `TestParseClaudeResetClock`, `TestParseClaudeResetClockRollover`, `TestParseClaudeResetClockRejectsInvalid`, `TestParseClaudeResetClockDST` |
| `.4`, `.5` | `routingerr/classify_test.go`: `TestClassifyClaudeResetHintPrecedence`, `TestClassifyClaudeInvalidResetClockKeepsQuotaClassification`, `TestClassifyClaudeStructuredHTTPStatusPrecedence`; existing Codex tests |
| `.1`, `.2`, `.6` | `transport/acp/opencode_stderr_test.go`: `TestProviderErrorFromErrorClaudeSessionLimit` |
| `.1`, `.2`, `.6` | `orchestrator/event_handlers_transient_claude_test.go`: `TestClassifyKanbanFailureClaudeSessionLimit`, `TestClaudeQuotaFixedProfileRemainsManual` |
| `.6` | `dynamic/engine_test.go`: `TestEngineRoutesClaudeSessionLimitThroughHardPolicy` |

Criterion numbers refer to `AC-AGENTS-CLAUDE-SESSION-LIMIT-001`.
`TestClassifyClaudeSessionLimit` must fail before the fix because the result is the unknown post-start rule.
`TestParseClaudeResetClock` must fail before the fix because the observed clock produces nil.

## End-to-end evidence

Use Go integration tests at the provider-error boundary instead of a browser scenario.
The ACP test projects a real `acp.RequestError` carrying the observed notice, then classifies the projected message.
The Kanban test exercises the existing failure classifier and fixed-profile policy.
The dynamic test exercises the classified notice, hard policy, shared-binding sibling exclusion, reset-time probe eligibility, and circuit closure after a successful probe.
No live subscription exhaustion or chargeable agent run is required.

## Work orders

- [x] [Task 01: Classify Claude session limits and parse reset clocks](task-01-classify-claude-session-limit.md)

## Verification results

Implementation checks on 2026-10-01:

- Initial red run: classifier returned `phase.poststart.unknown`; reset parser returned no hint, as expected.
- `go test ./internal/agent/runtime/routingerr -run 'TestClassifyClaudeSessionLimit$|TestParseClaudeResetClock$' -count=1`: passed.
- `go test ./internal/agent/runtime/routingerr ./internal/agent/runtime/dynamic -count=1`: passed.
- `go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestProviderErrorFromErrorClaudeSessionLimit$' -count=1`: passed.
- `go test -tags fts5 ./internal/orchestrator -run '^TestClassifyKanbanFailureClaudeSessionLimit$|^TestClaudeQuotaFixedProfileRemainsManual$' -count=1`: passed.
- `make -C apps/backend build`: passed. The macOS agentctl artifacts were built without codesign because neither `codesign` nor `rcodesign` was available.
- `python3 scripts/list-docs.py validate`: passed, 338 decisions and 1280 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation `validateCoverage` preflight: `covered`, no errors.
- `git diff --check`: passed.
- Review correction: the elapsed spring-gap regression now resolves `resets 3:30am (Europe/Helsinki)` at `2026-03-29T12:00:00Z` to `2026-03-30T00:30:00Z`; the upcoming spring-gap and autumn repeated-clock cases still return no hint.
- Post-correction `go test ./internal/agent/runtime/routingerr -run 'TestParseClaudeResetClock' -count=1`: passed.
- Post-correction `go test ./internal/agent/runtime/routingerr ./internal/agent/runtime/dynamic -count=1`: passed.

Design checks on 2026-10-01:

- `python3 scripts/list-docs.py validate`: passed, 338 decisions and 1280 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation `validateCoverage` preflight: `covered`, no errors.
  It included a prospective runtime change and all four linked package files.
- `git diff --check -- docs/specs docs/plans`: passed during package design.

The work order includes the exact preflight command for implementation delivery.

## Documentation impact

Internal specifications and delivery records change in this package.
Public recovery guidance already distinguishes quota failures and reset guidance.
No command, configuration, API, rendered card, or automatic-resume promise changes.
No new ADR is needed because the existing provider-neutral classification and validated-timing boundaries remain applicable.

## Risks

- Generic period matching also matches rate-limit wording. Keep the rule specific to the captured session notice.
- A selected future wall clock in a daylight-saving gap or repeated interval must produce no guessed deadline. If today's gap clock has already elapsed, advance to the next valid calendar day's clock.
- Embedded timezone data adds binary size and follows the Go toolchain's database version.
- Multi-component IANA names can be redacted before classification. Never relax diagnostic redaction to recover them.
- A reset deadline makes a circuit probe eligible; the existing exclusive probe remains active until its result closes or reopens the circuit.
- Quota classification alone does not authorize automatic replay or guarantee a rendered Claude reset field.
