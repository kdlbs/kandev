---
id: "03-durable-policy-evaluator"
title: "Durable policy evaluator"
status: done
wave: 3
depends_on: ["01-shared-error-catalogue", "02-versioned-policy-document"]
plan: "plan.md"
spec: "../../specs/platform/requirements/provider-error-recovery.md"
---

# Task 03: Durable policy evaluator

- **Acceptance:** Implement one provider-neutral evaluator that enforces effect
  safety, bounded reset waiting, exponential retry, and skip/stop exhaustion;
  persist its snapshot and deadline before scheduling; and reconcile timers
  safely across restart and generation races.
- **Files likely touched:** `apps/backend/internal/agent/runtime/dynamic/**`, a
  focused shared policy package under `apps/backend/internal/agent/runtime/`,
  `apps/backend/internal/task/repository/sqlite/{base_schema,base_migrations,dynamic_route}.go`,
  and focused tests using `testing/synctest`.
- **Dependencies:** Tasks 01 and 02.
- **Parallelism:** sequential runtime foundation.
- **Inputs:** Provider Error Recovery Policy evaluation, Durable scheduling,
  State machine, and Persistence guarantees; existing generation claims,
  circuit leases, and continuation effect evidence.
- **Output contract:** Report evaluation order, retry math, safety limits, wait
  ownership, persisted fields, restart reconciliation, race tests, files
  changed, exact commands/results, risks, and synchronized task/plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/dynamic/... ./internal/agent/runtime/routingerr/... ./internal/task/repository/sqlite/...`
- **Risks:** Timer/manual-action races can duplicate dispatch. Use one durable
  generation owner, injectable clocks, and no sleep-based tests. Reset waiting
  and its post-wait attempt do not consume exponential retries, and each class
  can wait at most once per candidate route cycle.

## Results

Completed. Added the shared versioned evaluator with effect-safety-first
ordering, one trusted reset wait per candidate and error class, exponential
retry with bounded delays, explicit skip/stop outcomes, and fail-closed
handling for unknown or unsafe failures. Dynamic route state now persists the
policy snapshot, failure code/class, catalogue version, retry ordinal,
deadline, reset-wait usage, and pending outcome. Automatic recovery re-arms
only undispatched waits after restart; ambiguous `retrying` states remain
manual. Manual retry, skip, cancel-wait, and stop transitions are generation
fenced.

Verification:

- `go test -tags fts5 ./internal/agent/runtime/dynamic/... ./internal/agent/runtime/routingpolicy ./internal/agent/runtime/routingerr/... ./internal/agent/settings/... ./internal/task/repository/sqlite/...` — 1,106 passed in 12 packages.
- `go test -tags fts5 ./internal/task/repository/sqlite -run 'Test(ListPendingRouteStates|DynamicRouteStateAndAttemptsPersistAcrossRepositoryReads)' -count=1` — 2 passed.
- `make -C apps/backend lint` — `golangci-lint`: 0 issues.
