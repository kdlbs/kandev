---
id: "07-operator-docs-load-validation"
title: "Document operations and validate sustained load"
status: pending
wave: 3
depends_on: ["06-database-maintenance-command"]
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 07: Document operations and validate sustained load

## Intent

Publish the safe upgrade/maintenance procedure and prove the combined change is stable under the production-like duplicate-watch load.

## Acceptance

- Public operations documentation describes backup of kandev.db and master.key, image upgrade, migration/maintenance dry run, execute/compact, rollback, and post-release health checks.
- A deterministic ten-minute multi-task/coordinator load test proves no watch-creation loop, one lookup per canonical identity, no recurring exhausted CAS errors, and stable API/history latency.
- Documentation identifies the maintenance command’s required backup and operator-selected retention, without claiming automatic deletion of conversations.

## Files likely touched

- docs/public/**/*.md
- docs/public/meta.json if a new page is needed
- apps/backend/AGENTS.md if command/metric conventions change
- apps/backend/internal/github/*_test.go
- apps/backend/internal/task/statussummary/*_test.go
- New bounded-load fixture/test package if existing integration tests cannot host it

## Dependencies

Task 06.

## Parallelism

Sequential. Documentation must describe the shipped command and load validation exercises the integrated behavior.

## Verification

~~~bash
cd apps/backend && go test ./internal/github ./internal/task/statussummary -run 'Test.*(Canonical|Load|TenMinute|Concurrent|CAS).*' -count=1 -v
cd apps/backend && go test ./internal/github ./internal/task/statussummary -count=1
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
~~~

## Output contract

Report the exact load fixture scale, lookup/watch/CAS/error/latency measurements, public-doc pages and Diátaxis type, validation outcomes, and upgrade/rollback instructions verified. Update task and plan status.

## Results

Pending.

