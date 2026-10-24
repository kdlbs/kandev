---
created: 2026-10-11
status: done
requirements:
  - REQ-AGENTS-CLAUDE-SESSION-LIMIT-001
system_design:
  - ../../specs/agents/system-design/claude-session-limit-classification.md
legacy_specs: []
---

# Implementation Plan: Claude Weekly-Limit Classification

## Overview

Claude ACP reports an exhausted weekly allowance with the same notice shape as
the session limit:

```text
Internal error: You've hit your weekly limit · resets 4am (<zone>)
```

The Claude rules only recognize the session-limit notice, so this failure is
classified as an unknown post-start runtime error. Dynamic routing then cannot
apply its hard-quota policy to the exhausted candidate.
AC-AGENTS-CLAUDE-SESSION-LIMIT-001.8 adds the weekly notice.

## Scope

### In scope

- Add `claude.stderr.weekly_limit.v1` next to the session-limit rule.
- Record the weekly evidence and its reset-hint boundary in the requirement and
  design.

### Out of scope

- Deriving a reset time from the weekly clock.
- Recovery policy or UI changes.

## Technical approach

The new rule mirrors the session-limit rule with the period word `weekly`: it
starts a line, allows an `Internal error:` or `Error:` prefix, accepts straight
and typographic apostrophes and `you have`, and ends at a word boundary. It
yields `CodeQuotaLimited` with `ConfHigh`. The clock-only reset parser stays
restricted to the session-limit rule because a weekly clock names no date; a
structured reset hint still takes precedence.

## Tests

`claude_weekly_limit_test.go` covers `TestClassifyClaudeWeeklyLimit`,
`TestClassifyClaudeWeeklyLimitDoesNotInventResetDate` and
`TestClassifyClaudeWeeklyLimitRejectsUnrelatedText`:
AC-AGENTS-CLAUDE-SESSION-LIMIT-001.8.

## E2E tests

Classifier tests cover the change. No rendered UI changes require browser
coverage.

## Work orders

- [x] [Task 01: Classify Claude weekly limits](task-01-classify-claude-weekly-limit.md)

## Verification results

The positive cases failed on the original rules and pass with the change.

## Risks

A future Claude notice that names the reset date could carry a usable hint; it
would need its own parser.
