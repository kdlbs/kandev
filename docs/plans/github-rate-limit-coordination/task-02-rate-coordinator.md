---
id: 02-rate-coordinator
title: Principal-wide request admission
status: done
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

- Shared observations and admission by normalized human login or App
  registration/installation, independently of workspace and credential
  generation.
- Direct HTTP, GraphQL, and `gh` execution now enforce retry windows before
  transport work; automated GitHub pollers use the background work class.
- Background calls serialize and pace per resource, retain a ten-percent
  primary reserve, and remain behind active interactive work.
- Focused and race validation passed with task-local Go caches.
