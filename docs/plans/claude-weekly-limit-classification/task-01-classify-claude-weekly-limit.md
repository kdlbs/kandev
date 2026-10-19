---
id: "01-classify-claude-weekly-limit"
title: "Classify Claude weekly limits"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CLAUDE-SESSION-LIMIT-001
acceptance_criteria:
  - AC-AGENTS-CLAUDE-SESSION-LIMIT-001.8
system_design:
  - ../../specs/agents/system-design/claude-session-limit-classification.md
---

# Task 01: Classify Claude weekly limits

## Summary

Classify Claude's weekly-limit notice as high-confidence `quota_limited`
without guessing a reset date from its clock.

## In scope

- Add classifier tests for the weekly notice before the rule.
- Add `claude.stderr.weekly_limit.v1`.
- Record the evidence and reset-hint boundary in the requirement and design.

## Out of scope

Reset-date parsing for weekly notices, recovery policy, and UI changes.

## Acceptance

- Weekly notices, with apostrophe, case and whitespace variants, classify as
  high-confidence hard `quota_limited` with fallback allowed.
- A weekly notice produces no text-derived reset hint; a structured hint is kept.
- Bare or approaching weekly-limit text, prose that only quotes the notice, and
  other providers do not acquire the classification; a structured HTTP status
  keeps precedence.

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
- apps/backend/internal/agent/runtime/routingerr/claude_weekly_limit_test.go
- docs/specs/agents/requirements/claude-session-limit-classification.md
- docs/specs/agents/system-design/claude-session-limit-classification.md

## Dependencies

None.

## Risks

A later notice format with a reset date would need its own parser.

## Parallelism

`sequential`

## Inputs

- REQ-AGENTS-CLAUDE-SESSION-LIMIT-001 and AC-AGENTS-CLAUDE-SESSION-LIMIT-001.8.
- The linked Claude session-limit classification design.

## Results

- RED: on the original rules, the weekly notices classified as
  `agent_runtime_error` (`phase.poststart.unknown`).
- GREEN: the new tests and the existing routing error tests pass, except
  `TestRemediateNpxCache_RemovesHashDirAndPreservesParent`, which fails on
  Windows without this change.
