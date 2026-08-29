---
id: 02-rate-coordinator
title: Principal-wide request admission
status: in_progress
wave: 2
depends_on: [01-rate-classification]
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-002
system_design: ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 02: Principal-Wide Request Admission

## Acceptance

- Workspaces with the same upstream principal share one tracker/admission state.
- Background work is paced, respects the ten-percent reserve, and yields to
  interactive work without holding capacity while delayed.
- Provider retry windows block admission and cancellation remains prompt.

## Verification

- `cd apps/backend && go test ./internal/github -run 'Test.*(Coordinator|Admission|Principal|Poller|RateTracker)' -count=1`
- `cd apps/backend && go test -race ./internal/github -run 'Test.*(Coordinator|Admission)' -count=1`

## Results

Pending.
