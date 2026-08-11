---
id: task-03-complete-path-bridge
title: Wire Auggie disk bridge on stream complete
status: done
wave: 3
depends_on: [task-02-disk-reader]
plan: docs/plans/auggie-disk-usage-bridge/plan.md
spec: docs/specs/office/costs.md
---

# Task 03 — Complete-path bridge

## Acceptance

- [x] `publishPromptUsage`: wire path unchanged when Usage non-nil.
- [x] When agent is auggie and Usage nil: resolve ACP id, read disk after watermark, publish summed `SessionPromptUsageEventPayload`, advance `auggie_usage_seq`.
- [x] Second complete does not republish same exchanges.
- [x] Non-auggie nil Usage still no-ops.
- [x] Tests use temp HOME + fixture session file + event bus capture.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/ -count=1 -run 'AuggieUsage|PromptUsage'
```

## Files

- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/auggie_usage_bridge_test.go` (new; keep streaming_test.go under line limits)
- `apps/backend/internal/task/models/models.go` (metadata key constant)
