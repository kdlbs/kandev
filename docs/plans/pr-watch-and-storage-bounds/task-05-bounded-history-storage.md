---
id: "05-bounded-history-storage"
title: "Bound history hydration and operational payload storage"
status: pending
wave: 2
depends_on: ["01-canonical-watch-migration"]
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 05: Bound history hydration and operational payload storage

## Intent

Keep normal history responsive while retaining conversations and making large operational data deduplicable and retention-eligible.

## Acceptance

- Every normal session-history hydration path uses cursor-bounded reads and does not eagerly materialize large tool metadata.
- Large tool payloads use digest-backed compressed/external storage; explicit authorized detail loading verifies integrity and ordinary user/agent messages remain retained.
- Equivalent Git snapshots deduplicate, while superseded payload/snapshot/plan-revision selection respects configurable retention and is non-destructive until maintenance executes.

## Files likely touched

- apps/backend/internal/task/models/message.go
- apps/backend/internal/task/repository/sqlite/message.go
- apps/backend/internal/task/repository/sqlite/git_snapshots.go
- apps/backend/internal/task/repository/sqlite/plan.go
- apps/backend/internal/task/repository/sqlite/base_schema.go
- apps/backend/internal/task/service/service_messages.go
- Relevant task controller/DTO hydration callers and focused repository/service tests

## Dependencies

Task 01 for shared migration/snapshot conventions.

## Parallelism

Sequential. It establishes the retention data contract consumed by the maintenance command.

## Verification

Build a multi-gigabyte-equivalent fixture through bounded blobs rather than committing a giant test database:

~~~bash
cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/service -run 'Test.*(Message|History|Payload|Snapshot|Retention|PlanRevision).*' -count=1 -v
cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/service -count=1
git diff --check
~~~

## Output contract

Report page limits, query/payload assertions, digest collision/dedup behavior, retention candidates, message-preservation evidence, and test outcomes. Update task and plan status.

## Results

Pending.

