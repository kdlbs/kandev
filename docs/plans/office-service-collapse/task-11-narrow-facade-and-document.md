---
id: "11-narrow-facade-and-document"
title: "Narrow the facade's exported surface and document the boundary"
status: pending
wave: 5
depends_on: ["02-config-helpers", "03-config-import-drift", "04-projects-domain", "05-agents-domain", "06-skills-routines", "07-channels-comments", "08-activity-onboarding", "09-scheduler-run-claim"]
plan: "plan.md"
spec: none
parallel-safe: false
---

# Task 11: Narrow the Facade and Record the Boundary

Closes the plan. Without this task the mirror grows back — nothing in the repo
would state that `office/service` is not where office CRUD goes.

Deliberately does **not** depend on task 10: the retry fork can land later
without holding up the boundary documentation.

## Scope

### 1. Narrow the exported surface

After tasks 02–09, `*service.Service`'s live external surface should be exactly
the 17 methods enumerated in [`inventory.md`](inventory.md) §"Live consumers":

- `tree_controls` — `CancelTaskTree` `PauseTaskTree` `ResumeTaskTree`
  `RestoreTaskTree` `PreviewTaskTree` `GetSubtreeCostSummary`
- `workspaces` — `DeleteWorkspace` `GetWorkspaceDeletionSummary`
- `scheduler` — `BuildWakePayload` `ResolveExecutor` (the other four are
  repointed by tasks 05 and 08)
- `backendapp` — `SetWorkflowEngineDispatcher` `ListFailedRunInboxRows`
  `ListPausedAgentInboxRows` plus the `Set*` / `Register*` wiring calls

Unexport anything left over that no longer has an external caller. Re-run
`deadcode ./...` and clear what it reports in `office/service` — the pre-existing
`IdleTimeoutManager` cluster (`idle_timeout.go:38-161`, 8 unreachable funcs) and
`TickIntervalFromEnv` are **out of scope**; report them as a follow-up finding
rather than deleting them here, since they are unrelated to the collapse.

### 2. Record the boundary

Update `apps/backend/AGENTS.md`, in the `internal/office/` section:

> `internal/office/service` owns **run execution only** — the tick/dispatch
> loop, prompt and env construction, executor resolution, wake payloads, failure
> and retry handling, task-tree controls, and workspace deletion. Domain CRUD,
> config import/export, and every HTTP route belong to the feature sub-packages
> (`agents`, `skills`, `projects`, `routines`, `config`, `channels`, `costs`,
> `approvals`, `labels`, `dashboard`, `onboarding`). `office/service` has no
> `gin` import and must not gain one. New office CRUD goes in the sub-package,
> never in `service`.

Then land the ADR at
`docs/decisions/2026-08-08-office-domain-ownership-boundary.md` (drafted with
this plan) and add its row to `docs/decisions/INDEX.md`.

### 3. Close the loop on the detector

Record the final Section A and Section B counts in
[`inventory.md`](inventory.md) as an "after" column beside the 2026-08-08
baseline, so the next person can tell what this plan removed from what it left.

Expected end state: Section A **40 → 2** (the out-of-scope
`taskIDFromPayload`↔`extractRunTaskID` dashboard↔scheduler pair, plus
`retryDelayWithJitter` if task 10 has not landed). Section B same-name pairs
**37 → 3** or fewer.

## Acceptance

1. `apps/backend/AGENTS.md` states the ownership boundary.
2. The ADR is committed and indexed in `docs/decisions/INDEX.md`.
3. `inventory.md` carries before/after counts.
4. `grep -rn 'gin\.' apps/backend/internal/office/service/` returns nothing.
5. Every remaining exported method on `*service.Service` has a named external
   caller, or is documented as intentionally exported for test support.

## Verification

```bash
grep -rn 'gin\.' apps/backend/internal/office/service/    # expect no hits
cd apps/backend && ~/go/bin/deadcode ./... | grep 'office/service'
cd apps/backend && wc -l $(find internal/office/service -name '*.go' -not -name '*_test.go') | tail -1

make -C apps/backend test
make -C apps/backend lint
cd apps/backend && golangci-lint run ./... --new-from-rev=main --timeout=5m

cd docs/plans/office-service-collapse/officedup && GOTOOLCHAIN=local go run . \
  ../../../../apps/backend/internal/office | head -3
```

## Files likely touched

- `apps/backend/AGENTS.md`
- `docs/decisions/2026-08-08-office-domain-ownership-boundary.md`
- `docs/decisions/INDEX.md`
- `docs/plans/office-service-collapse/inventory.md`, `plan.md`
- `internal/office/service/*.go` (unexporting leftovers)

## Dependencies

Tasks 02–09. Explicitly **not** task 10.

## Parallelism

`sequential`.

## Rollback position

Docs-only except the unexporting. If an unexport breaks an unforeseen caller,
revert that symbol alone — the AGENTS.md and ADR changes stand independently.

## Output contract

Summary, files changed, final `office/service` LOC against the 7,818 baseline,
before/after detector counts, the `deadcode` residue reported as follow-up, and
the ADR link.

## Results

Pending.
