---
id: "01-classify-opencode-credit-exhaustion"
title: "Classify OpenCode credit and billing diagnostics"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-CREDIT-001
acceptance_criteria:
  - AC-AGENTS-OPENCODE-CREDIT-001.1
  - AC-AGENTS-OPENCODE-CREDIT-001.2
  - AC-AGENTS-OPENCODE-CREDIT-001.3
  - AC-AGENTS-OPENCODE-CREDIT-001.4
  - AC-AGENTS-OPENCODE-CREDIT-001.5
  - AC-AGENTS-OPENCODE-CREDIT-001.6
system_design:
  - ../../specs/agents/system-design/opencode-credit-classification.md
---

# Task 01: Classify OpenCode credit and billing diagnostics

## Summary

Classify explicit OpenCode credit exhaustion as quota and payment-only text as
a user-action billing condition. Preserve precedence for combined text and
structured status.

## In scope

- Add bounded quota and payment rules to the `opencode-acp` provider catalogue.
- Add tests for recognized phrases, unrelated mentions, priority, and flags.
- Store the end-to-end design and acceptance criteria for these behaviors.

## Out of scope

- Changing classification rules for other providers.
- Parsing reset times from these diagnostics.
- Changing candidate order, credential circuits, or provider selection.

## Acceptance

- `AC-AGENTS-OPENCODE-CREDIT-001.1` through
  `AC-AGENTS-OPENCODE-CREDIT-001.6` describe the behaviors covered by this
  work order.

## Verification

```bash
cd apps/backend && go test ./internal/agent/runtime/routingerr -count=1
cd apps/backend && golangci-lint run ./... --new-from-rev=ff917a370ac9127fee6f8d7cee80b6b7b9b6175b --timeout=5m
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

- `opencode.stderr.credit.v1` classifies explicit credit exhaustion as
  high-confidence `quota_limited`.
- `opencode.stderr.subscription.v1` classifies payment-only text as
  high-confidence `subscription_required`.
- Table-driven tests cover all accepted phrases, negative credit mentions,
  provider scope, overlapping signals, structured status priority, and flags.
- `go test ./internal/agent/runtime/routingerr -count=1` passed.
- `golangci-lint run ./... --new-from-rev=ff917a370ac9127fee6f8d7cee80b6b7b9b6175b --timeout=5m`
  passed with 0 issues.
- Specification validation and document catalog validation passed after docs
  changes.
