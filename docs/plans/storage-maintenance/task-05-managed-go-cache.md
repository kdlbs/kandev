---
id: "05-managed-go-cache"
title: "Managed Go build cache"
status: done
wave: 2
depends_on: ["02-storage-persistence"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 05: Managed Go build cache

Own one explicit Go build cache for local Kandev executions and rotate it safely above threshold.

## Acceptance

- Enabling the provider injects one absolute managed `GOCACHE` into setup, cleanup, shell, agent,
  test, and build processes for every new local execution.
- Analysis and cleanup operate only on a marked Kandev path or explicitly confirmed adopted path;
  default user caches remain untouched.
- Above-threshold idle cleanup quarantines the cache, recreates an empty writable directory, and
  reports byte deltas; disabling injection does not implicitly delete old data.

## Verification

```bash
cd apps/backend && go test ./internal/system/storage/gocache ./internal/agent/runtime/lifecycle
```

## Files likely touched

- `apps/backend/internal/system/storage/gocache/provider.go`
- `apps/backend/internal/system/storage/gocache/provider_test.go`
- lifecycle environment construction and script/shell propagation files
- focused lifecycle environment tests

## Dependencies

- Task 02 for settings and quarantine records.

## Inputs

- Spec Go build cache section
- Existing agent/executor profile environment merge order
- Go's absolute `GOCACHE` contract

## Output contract

Report path ownership and adoption safeguards, propagation call sites, cleanup behavior, tests run,
blockers/risks, and update this task plus `plan.md` to done.
