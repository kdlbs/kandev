---
status: done
created: 2026-09-30
requirements:
  - REQ-AGENTS-OPENCODE-CREDIT-001
system_design:
  - ../../specs/agents/system-design/opencode-credit-classification.md
---

# OpenCode credit-exhaustion classification

## Overview

OpenCode can report an exhausted credit allowance or a payment problem as text.
The classifier must distinguish these causes so route recovery can skip an
exhausted allowance and surface a billing problem.

## Technical approach

Keep the rules in the existing `opencode-acp` provider table. Match explicit
credit-exhaustion phrases as `quota_limited`. Match payment-only text as
`subscription_required`. Preserve structured status and rule-order precedence.
Test the positive phrases, negative boundaries, provider scope, and recovery
flags in `routingerr.Classify`.

## Delivery order

1. [Task 01: Classify OpenCode credit and billing diagnostics](task-01-classify-opencode-credit-exhaustion.md)

## Verification strategy

- Run `go test ./internal/agent/runtime/routingerr -count=1` in `apps/backend`.
- Run backend lint with the PR base SHA.
- Run the specification and document catalog checks after doc edits.
