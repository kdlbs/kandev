---
id: "01-classify-opencode-go-usage-limit"
title: "Classify the OpenCode Go usage limit"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-CREDIT-001
acceptance_criteria:
  - AC-AGENTS-OPENCODE-CREDIT-001.7
system_design:
  - ../../specs/agents/system-design/opencode-credit-classification.md
---

# Task 01: Classify the OpenCode Go usage limit

## Summary

Classify OpenCode Go's `Go usage limit exceeded` notice as high-confidence
`quota_limited`.

## In scope

- Add classifier tests for the notice before the rule.
- Add `opencode.acp.go_usage_limit.v1`.
- Record the notice in the requirement and design.

## Out of scope

Reset-time parsing, other providers, and recovery policy.

## Acceptance

- The notice, with or without an error prefix and in any case, classifies as
  high-confidence hard `quota_limited` with fallback allowed and no reset hint.
- A correlated period notice with a reset time keeps the period rule and hint.
- Prose that only quotes the notice, a bare phrase, and other providers do not
  acquire the rule.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/routingerr/... -count=1)
(cd apps/backend && go vet -trimpath ./internal/agent/runtime/routingerr/...)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/agent/runtime/routingerr/rules.go
- apps/backend/internal/agent/runtime/routingerr/opencode_go_usage_limit_test.go
- docs/specs/agents/requirements/opencode-credit-classification.md
- docs/specs/agents/system-design/opencode-credit-classification.md

## Dependencies

None.

## Risks

Low.

## Parallelism

`sequential`

## Inputs

- REQ-AGENTS-OPENCODE-CREDIT-001 and AC-AGENTS-OPENCODE-CREDIT-001.7.
- The linked OpenCode credit classification design.

## Results

- RED: on the original rules, the Go usage-limit notices did not classify as
  `quota_limited` with this rule.
- GREEN: the new tests and the existing routing error tests pass, except
  `TestRemediateNpxCache_RemovesHashDirAndPreservesParent`, which fails on
  Windows without this change.
