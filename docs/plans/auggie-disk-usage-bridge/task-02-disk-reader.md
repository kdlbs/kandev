---
id: task-02-disk-reader
title: TDD Auggie session-file usage reader
status: done
wave: 2
depends_on: [task-01-spec-amend]
plan: docs/plans/auggie-disk-usage-bridge/plan.md
spec: docs/specs/office/costs.md
---

# Task 02 — Disk reader

## Acceptance

- [x] `ReadNewExchangeUsages` returns only sequenceId, model_id, finishedAt, token ints.
- [x] Filters `completed==true` and `sequenceId > after`.
- [x] Sums all non-nil `response_nodes[].token_usage` per exchange.
- [x] Missing file / corrupt JSON soft errors; fixtures use scrubbed content.
- [x] Helper for Augment sessions path under HOME.

## Verification

```bash
cd apps/backend && go test ./internal/agent/auggieusage/ -count=1
```

## Files

- `apps/backend/internal/agent/auggieusage/session_file.go`
- `apps/backend/internal/agent/auggieusage/session_file_test.go`
- `apps/backend/internal/agent/auggieusage/testdata/*.json`
