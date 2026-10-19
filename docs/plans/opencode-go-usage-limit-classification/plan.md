---
created: 2026-10-12
status: done
requirements:
  - REQ-AGENTS-OPENCODE-CREDIT-001
system_design:
  - ../../specs/agents/system-design/opencode-credit-classification.md
legacy_specs: []
---

# Implementation Plan: OpenCode Go Usage-Limit Classification

## Overview

OpenCode Go reports an exhausted subscription through the ACP prompt error as
`AI_APICallError: Go usage limit exceeded`, without a reset time. Neither the
OpenCode period usage-limit rule nor the generic quota rule matches it, so the
failure classifies as an unknown runtime error and dynamic routing cannot apply
its hard-quota policy. AC-AGENTS-OPENCODE-CREDIT-001.7 adds the notice.

## Scope

### In scope

- Add `opencode.acp.go_usage_limit.v1` after the period usage-limit rule.
- Record the notice in the OpenCode credit classification requirement and
  design.

### Out of scope

- Reset-time parsing for this notice.
- Other providers and recovery policy.

## Technical approach

The rule uses the line-start shape of the other OpenCode rules, allowing an
`AI_APICallError:`, `Error:`, `Internal error:` or `API Error:` prefix, and
yields `CodeQuotaLimited` with `ConfHigh`. It runs after the period rule, so a
stderr notice that also carries a reset time keeps that rule and its hint.

## Tests

`opencode_go_usage_limit_test.go` covers
`TestClassify_OpenCodeGoUsageLimitExceededIsHighConfidenceQuota`,
`TestClassify_OpenCodeGoUsageLimitKeepsPeriodNoticeResetHint` and
`TestClassify_OpenCodeGoUsageLimitRejectsUnrelatedText`:
AC-AGENTS-OPENCODE-CREDIT-001.7.

## E2E tests

Classifier tests cover the change. No rendered UI changes require browser
coverage.

## Work orders

- [x] [Task 01: Classify the OpenCode Go usage limit](task-01-classify-opencode-go-usage-limit.md)

## Verification results

The positive cases failed on the original rules and pass with the change.

## Risks

Low: one provider-scoped rule with the shape of the existing OpenCode rules.
