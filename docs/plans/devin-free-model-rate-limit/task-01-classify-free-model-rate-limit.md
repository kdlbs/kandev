---
id: "01-classify-free-model-rate-limit"
title: "Classify the Devin free-model rate limit"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
acceptance_criteria:
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.13
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
---

# Task 01: Classify the Devin free-model rate limit

## Summary

A `devin-acp` diagnostic line that opens with `Reached free model rate limit`
classifies as high-confidence `rate_limited` with fallback allowed, and its
`(at HH:MM <zone>)` clock becomes an inferred reset hint on the right day.

## In scope

- `ruleDevinFreeModelRateLimit` and `providerDevinACP` in `rule_ids.go`.
- The `devin-acp` entry in `providerRules`, built with
  `mustRuleOutsideMarkdownFences`, and the rule in `ruleUsesScopedTextReset`.
- `parseResetAtClockHintAt`, `parseResetRelativeDuration`,
  `resolveResetAtClock`, and `nextDailyResetAt` (extracted from
  `parseResetClockHintAt` without behavior change) in `resethint.go`.
- The rule-specific reset branch in `Classify`.
- Tests in a new file `devin_free_model_limit_test.go`.

## Out of scope

- Other providers' rules, rule order, and flags.
- Routing, circuit, and recovery policy.

## Acceptance

1. The notice with `in 8 minutes`, `in 1 hour 12 minutes`, `in 5 minutes`, and
   an `Error:` label classifies as high-confidence transient `rate_limited`
   with fallback allowed in the prompt-send and streaming phases, with an
   inferred reset at the stated clock.
2. Relative duration selects the clock occurrence closest to the estimated
   reset date, including the prior date when the clock elapsed just after
   midnight and dates beyond the next calendar day. Without a usable duration,
   a clock-only notice selects the first occurrence after observation time.
3. Prose, mid-sentence quotes, fenced examples, incomplete notices, a missing
   word boundary, and other providers stay unmatched; an unrelated clock on
   another line never becomes the reset.
4. A structured HTTP 429 for `devin-acp` keeps the `http.429` classification.
   Without an explicit reset hint, only a matched Devin notice may supply an
   inferred reset. An explicit reset hint keeps precedence.

## Verification

Write the tests first and confirm acceptance 1 and 2 fail on the phase
default before the change.

```bash
# from the repository root
(cd apps/backend && go test -trimpath -tags fts5 -count=1 \
  -run 'Devin' ./internal/agent/runtime/routingerr/)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 \
  ./internal/agent/runtime/routingerr/)
bash scripts/lint-go-changed \
  apps/backend/internal/agent/runtime/routingerr/resethint.go \
  apps/backend/internal/agent/runtime/routingerr/routingerr.go \
  apps/backend/internal/agent/runtime/routingerr/rule_ids.go \
  apps/backend/internal/agent/runtime/routingerr/rules.go \
  apps/backend/internal/agent/runtime/routingerr/devin_free_model_limit_test.go
```

## Likely files

- `apps/backend/internal/agent/runtime/routingerr/rule_ids.go`
- `apps/backend/internal/agent/runtime/routingerr/rules.go`
- `apps/backend/internal/agent/runtime/routingerr/routingerr.go`
- `apps/backend/internal/agent/runtime/routingerr/resethint.go`
- `apps/backend/internal/agent/runtime/routingerr/devin_free_model_limit_test.go` (new)

## Results

- Before the change, the four positive tests failed: the notice classified as
  `agent_runtime_error` through `phase.poststart.unknown` with no reset hint.
- With the rule and parser, every `Devin` test passes, and the rest of the
  package keeps passing.
- On a Windows host, `TestRemediateNpxCache_RemovesHashDirAndPreservesParent`
  fails before and after the change because its fixture path sits outside the
  host npm cache root; it is unrelated to this work order.
