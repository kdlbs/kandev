---
created: 2026-11-19
status: implemented
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
legacy_specs: []
---

# Implementation plan: Devin free-model rate limit classification

## Overview

`devin-acp` has no provider rule. Its free-plan notice ("Reached free model
rate limit. ... Your limit will reset in 8 minutes (at 12:01 UTC)") therefore
classifies as the phase default: `agent_runtime_error`, low confidence, no
fallback. Dynamic routing keeps sending prompts to the throttled candidate
instead of suspending it until the stated reset and selecting another one.
This package adds a scoped `devin-acp` rate-limit rule and derives the reset
from the notice, as defined by `AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.13` and
the "Devin free-model rate limit" section of the system design.

## Scope

In scope: the `devin-acp` rule, the clock-with-duration reset parser, inferred
reset hints for a matching Devin notice with structured HTTP 429, and classifier
tests (positive variants, day rollover, relative-duration disambiguation, prose
and fence negatives, other providers, line-scoped reset).

Out of scope: rule order or flags of other providers, routing policy, circuit
handling, and any frontend change.

## Work orders

- [x] [Task 01: Classify the Devin free-model rate limit](task-01-classify-free-model-rate-limit.md)

One work order: one rule plus its reset parser, with one verification boundary.

## Technical approach

`routingerr.Classify` keeps structured HTTP classification and explicit reset
hints authoritative. When a structured HTTP 429 has no reset hint, the exact
matched, unfenced Devin diagnostic line can supply an inferred clock reset.
For a relative duration, the parser estimates the reset time as observation
time plus that duration. It compares the clock on the estimated local date and
the adjacent calendar dates in the notice's time zone. Without a valid
duration, it uses the first clock occurrence after the observation time.

The existing routing and recovery consumers already use `ResetHint`, so this
change adds no new routing policy or state owner.

## Tests

`AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.13` is covered by these tests in
`apps/backend/internal/agent/runtime/routingerr`:

- `TestClassifyDevinFreeModelRateLimit` checks the positive notice forms,
  classification flags, and inferred reset.
- `TestClassifyDevinFreeModelRateLimitRollsClockToNextDay` checks clock-only
  behavior after the clock has passed.
- `TestClassifyDevinFreeModelRateLimitUsesRelativeDuration` checks elapsed
  clocks, midnight rollover, a clock that has just passed before midnight, and
  a duration whose target is beyond the next calendar date.
- `TestClassifyDevinFreeModelRateLimitRejectsUnrelatedText` checks prose,
  fenced examples, incomplete notices, and other providers.
- `TestClassifyDevinFreeModelRateLimitBindsResetToDiagnosticLine` checks that
  only the matched unfenced line supplies a reset.
- `TestClassifyDevinFreeModelRateLimitUsesMatchedLineForStructuredHTTP429`
  checks that a structured 429 retains its classification, infers only from a
  matched Devin line, and preserves an explicit reset hint.

No end-to-end test is needed. The result is a classification and reset hint
that existing routing and recovery consumers already handle for other
providers.

## Verification results

- `cd apps/backend && go test -trimpath -tags fts5 -count=1 -run 'Devin'
  ./internal/agent/runtime/routingerr/`: passed.
- `cd apps/backend && go test -trimpath -tags fts5 -count=1
  ./internal/agent/runtime/routingerr/`: passed.
- `bash scripts/lint-go-changed` with the four changed Go files: passed with
  zero issues.
- `python3 scripts/list-docs.py validate`: passed; 456 decisions and 2045
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

## Risks

- The notice wording can change. An unmatched variant falls back to the
  current phase default, so the failure mode is today's behavior.
- A wrong day choice could park a candidate for a day. The relative duration
  disambiguates the day, and an elapsed hint only shortens the suspension.

## References

- [Requirement](../../specs/agents/requirements/provider-error-signatures.md)
- [System design](../../specs/agents/system-design/provider-error-signatures.md)
