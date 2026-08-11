---
id: task-04-verify
title: Verify Auggie usage bridge tests
status: done
wave: 4
depends_on: [task-03-complete-path-bridge]
plan: docs/plans/auggie-disk-usage-bridge/plan.md
spec: docs/specs/office/costs.md
---

# Task 04 — Verify

## Acceptance

- [x] Reader package tests pass.
- [x] Orchestrator Auggie bridge tests pass.
- [x] gofmt clean on touched Go files.

## Verification

```bash
cd apps/backend && go test ./internal/agent/auggieusage/ ./internal/orchestrator/ -count=1 -run 'Auggie|PromptUsage|auggie'
gofmt -l internal/agent/auggieusage internal/orchestrator/event_handlers_streaming.go internal/task/models/models.go
```
