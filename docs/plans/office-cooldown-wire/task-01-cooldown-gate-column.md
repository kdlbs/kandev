---
id: "01-cooldown-gate-column"
title: "Wire the heartbeat cooldown gate to office_agent_runtime"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/agents.md"
---

# Task 01: Wire the heartbeat cooldown gate to `office_agent_runtime`

## Root cause

`heartbeatAgentRuntime.AllowFire` (`apps/backend/internal/backendapp/cron.go:160-184`) loads the
agent via `r.office.GetAgentInstance(ctx, agentID)` and gates on `agent.LastRunFinishedAt`. That
field is `models.AgentInstance.LastRunFinishedAt`, which maps to `agent_profiles.last_run_finished_at`
— a column owned by the shared kanban agent-settings store
(`internal/agent/settings/store/sqlite.go`). No Office write path ever sets that field (confirmed:
`grep -rn '\.LastRunFinishedAt\s*=' apps/backend/internal` outside test files matches nothing), so
it round-trips as whatever the row already had — effectively always NULL for Office agents. The gate
condition `agent.LastRunFinishedAt != nil && agent.CooldownSec > 0` is therefore always false and the
cooldown branch never executes.

PR #2902 correctly and centrally stamps `office_agent_runtime.last_run_finished_at` (via
`Service.stampRunFinished` -> `Repository.UpdateRuntimeLastRunFinished`) on every completed/stopped
run. That is the column the spec (`docs/specs/office/agents.md` persistence guarantees, and now this
plan's spec amendment) documents as the cooldown guard's source of truth. `office.GetAgentRuntime(ctx,
agentID)` (`internal/office/repository/sqlite/runtime.go:20`) already reads it and returns
`*RuntimeState{LastRunFinishedAt *time.Time}`, `nil, nil` when the agent has no runtime row yet.

## Acceptance

- `heartbeatAgentRuntime.AllowFire` gates on `office_agent_runtime.last_run_finished_at` (via
  `office.GetAgentRuntime`), not `agent_profiles.last_run_finished_at`.
- `agent.CooldownSec` (from `agent_profiles`, via `GetAgentInstance`) is still the cooldown duration
  source — unchanged.
- No runtime row (`GetAgentRuntime` returns `nil, nil`) or `LastRunFinishedAt == nil` on the runtime
  row still allows firing (an agent that has never run is not gated) — same behavior as today, just
  reading the right column.
- `CooldownSec <= 0` still skips the runtime lookup entirely and always allows firing — preserve the
  existing short-circuit / avoid an unconditional extra query.
- Update the doc comment above `heartbeatAgentRuntime` (lines ~149-155) to name the column
  explicitly instead of the generic "LastRunFinishedAt" it currently says.

## Regression test

Add `apps/backend/internal/backendapp/cron_test.go` (package `backendapp`, same package as
`cron.go` — `heartbeatAgentRuntime` is unexported). Follow the harness pattern already used in
`adapters_office_test.go`'s `newOfficeTaskAdapterHarness` (`db.OpenSQLite` + `officesqlite.NewWithDB`)
to build a real `*officesqlite.Repository` — this is a repository-column bug, so exercise the actual
SQL, not a fake.

Test (must fail before the fix, pass after):

1. Create an `AgentInstance` with `CooldownSec: 60`, `Status: idle`.
2. Stamp `office_agent_runtime.last_run_finished_at` to `time.Now().UTC()` via
   `repo.UpdateRuntimeLastRunFinished(ctx, agent.ID, now)`.
3. Separately write a stale/irrelevant value into `agent_profiles.last_run_finished_at` for the same
   agent (e.g. `repo.UpdateAgentInstance` with `LastRunFinishedAt: &tenMinutesAgo`, or raw SQL) to
   prove the gate does not read that column — pre-fix this would make the test pass for the wrong
   reason if the assertion only checked "blocked", so assert specifically that firing is blocked
   despite `agent_profiles.last_run_finished_at` being far outside any cooldown window.
4. Call `(&heartbeatAgentRuntime{office: repo}).AllowFire(ctx, agent.ID, now.Add(30*time.Second))`
   (30s into a 60s cooldown) and assert `false, nil`.
5. Call `AllowFire` again with `now.Add(61*time.Second)` and assert `true, nil` (cooldown elapsed).
6. A second case with no `office_agent_runtime` row at all (skip step 2) asserts `true, nil` —
   never-run agents are not gated.

## Verification

```bash
cd apps/backend && go test ./internal/backendapp/... -run TestHeartbeatAgentRuntime -v
cd apps/backend && go test ./internal/backendapp/...
cd apps/backend && golangci-lint run ./internal/backendapp/... --new-from-rev=HEAD~1 --timeout=5m
```

## Files likely touched

- `apps/backend/internal/backendapp/cron.go`
- `apps/backend/internal/backendapp/cron_test.go` (new)

## Dependencies

None.

## Parallelism

Sequential. Marked parallel-safe with Task 02 at the plan level (disjoint files/tests, no shared
schema) but this repo's convention keeps `parallelism: sequential` as the per-task default; delegate
only on explicit user authorization.

## Inputs

- Spec: `docs/specs/office/agents.md` persistence-guarantees bullet (amended by this plan).
- Existing pattern: `apps/backend/internal/office/repository/sqlite/runtime_test.go` for
  `GetAgentRuntime`/`UpdateRuntimeLastRunFinished` behavior; `adapters_office_test.go` for the
  `backendapp`-package office-repo test harness.

## Risks

- `GetAgentInstance` and `GetAgentRuntime` are two separate queries per `AllowFire` call (previously
  one). This runs once per agent per heartbeat cron tick (`DefaultTickInterval`), not per-task —
  negligible load; do not attempt to fold them into a JOIN as part of this fix.
- Do not touch `agent_profiles.last_run_finished_at` itself (no migration, no field removal) — out
  of scope per the plan; it's dead for Office cooldown purposes but may still round-trip harmlessly
  through the shared kanban settings store.

## Output contract

Report the exact `AllowFire` behavior before/after, files changed, exact commands and results,
blockers/risks, then mark this task `done` and update its checkbox in `plan.md`.

## Completion report

- **Before**: `AllowFire` gated on `agent.LastRunFinishedAt` (`agent_profiles.last_run_finished_at`
  via `GetAgentInstance`), a column no Office write path ever stamps — the cooldown branch never
  fired. Reproduced by `TestHeartbeatAgentRuntime_AllowFire_ReadsOfficeRuntimeCooldown`, which failed
  pre-fix with `AllowFire (within cooldown) = true, want false`.
- **After**: `AllowFire` short-circuits to `true` when `CooldownSec <= 0` (unchanged), otherwise loads
  `office_agent_runtime` via `office.GetAgentRuntime` and gates on `runtime.LastRunFinishedAt`. No
  runtime row, or a nil `LastRunFinishedAt`, still allows firing (never-run agent not gated).
  `agent_profiles.last_run_finished_at` is no longer read by the gate at all.
- Files changed: `apps/backend/internal/backendapp/cron.go` (production fix + doc comment),
  `apps/backend/internal/backendapp/cron_test.go` (new — two regression tests).
- Commands and results:
  - `go test ./internal/backendapp/... -run TestHeartbeatAgentRuntime -v` — fails before the fix
    (`within cooldown` case wrongly allows), passes after (both tests green).
  - `go test ./internal/backendapp/...` — `ok` (all packages).
  - `golangci-lint run ./internal/backendapp/... --new-from-rev=HEAD --timeout=5m` — `0 issues`.
- Blockers/risks: none. Confirmed two extra one-time setup steps needed in the test harness beyond
  the plan's sketch: the shared `agent_profiles`/`agents` schema is owned by
  `internal/agent/settings/store` (not the office or task repos), and `agent_profiles.agent_id` has a
  FK into `agents` that required seeding one row — both are test-only harness details, no production
  code implication.
