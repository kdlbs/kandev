---
spec: none (behavior-preserving refactor — /spec excludes refactors)
adr: docs/decisions/2026-08-08-office-domain-ownership-boundary.md
created: 2026-08-08
status: draft
---

# Implementation Plan: Collapse `internal/office/service` Into the Office Sub-Packages

## Overview

`internal/office/service` (7,818 LOC, 38 non-test files) carries a verbatim CRUD
and config mirror of `office/{agents,channels,config,projects,routines,skills}`,
plus a third fork of `office/scheduler`'s run-claim and retry logic. Both copies
are live in the binary. This plan removes the mirror in **10 independently
mergeable PRs**, sequenced identical-bodies-first, and narrows `office/service`
to what it is actually used for: the run-execution engine behind
`tree_controls`, `workspaces`, `scheduler`, and `backendapp`.

No behavior changes are intended except the six documented in
[`inventory.md`](inventory.md) §"The 6 differences that are real", each of which
picks a side deliberately and lands with a test that pins it.

## Evidence base

[`inventory.md`](inventory.md) — regenerated 2026-08-08 against `fb1d8fdcd` with
an AST-based detector committed at [`officedup/`](officedup/). **Re-run it before
starting any task**; every task's acceptance includes a group-count delta.

Headline corrections to the assumptions this plan was commissioned under:

- **40 identical-body groups, not 33 names.** The 8 extra were missed because
  the text heuristic keys on function name and does not normalize the receiver
  identifier. Two of them (`generateSlug`, `writeWorkspaceConfig`) put
  `office/onboarding` in scope, and one (`ListActivityFiltered`) puts
  `office/dashboard` in scope. Neither was in the original brief.
- **31 of the 37 drifted pairs are cosmetic, and similarity score does not rank
  risk.** The four pairs flagged as high-risk (`ApplyImport` 0.990,
  `CreateProject` 0.985, `validateProject` 0.974, `applyAgents` 0.964) are —
  except for `applyAgents` — receiver renames over a `type Project =
  models.Project` alias. The two genuinely dangerous differences score *lowest*:
  `validateAgentNameUnique` at 0.845 and `scheduleRetry` at 0.745.
- **The collapse direction is the same for all six domains**, contrary to the
  survey's expectation. See below.

## Survey results

### 1. Direction of collapse — sub-packages win, uniformly

The deciding evidence is not LOC or importer count, it is the composition root.
`internal/office/service` has **no `gin` import and no HTTP surface**.
`office/routes.go` builds every handler from the sub-package services, and
`office/services.go:26-40` wires `Agents`/`Skills`/`Projects`/`Routines`/
`Config`/`Channels` to sub-package types. Even the `office/runtime` action
interfaces (`Projects.CreateProject`, `Skills.DeleteSkill`,
`AgentModifier.UpdateAgentInstance`) resolve to `svcs.Projects`/`svcs.Skills`/
`svcs.Agents`, not to the facade.

So for **agents, channels, config, projects, routines, skills** the answer is the
same: **the sub-package is the owner; the facade's copy is deleted**, not made to
delegate. The facade's mirror is reachable only from inside `service`, from its
own tests, and from six `scheduler` calls (task 05 repoints three of them).

This is not the "it will not be the same answer everywhere" the brief expected,
and the reason is worth stating: the extraction did complete on the *routing*
side. What it never did was delete the source. The facade is not a facade — it is
an orphaned copy that only its own test suite still exercises.

The two apparent counter-examples are not counter-examples:

- **`office/workspaces` (61 LOC)** and **`office/tree_controls`** are not forks.
  They are thin gin handlers *over* `*service.Service`
  (`workspaces/handler.go:13`, `tree_controls/handler.go:12`). They contain no
  duplicated logic and define the 8-method run-engine surface the facade keeps.
- **`office/skills` (2,332 LOC)** is the largest sub-package, but `office/service`
  **already imports it** (`service/skill_compat.go` re-exports
  `skills.AgentTypeResolver`, `skills.ParseDesiredSlugs`, and the source-type
  constants). The dependency edge already points the right way.

### 2. Is `office/scheduler` a third fork? — Yes; scoped in, split across two tasks

Confirmed. `ClaimNextRun` and `retryDelayWithJitter` are byte-identical across
`service/scheduler_runs.go`/`service/retry.go` and `scheduler/`; `ProcessRunGuard`
(0.979), `guardAgentStatus` (0.978), `escalateFailure` (0.982),
`queueCEOAgentError` (0.960) and `scheduleRetry` (0.745) are near-identical.

Direction here is forced and runs opposite to the six CRUD domains:
`office/scheduler` **imports** `office/service` (`scheduler/run.go:154`
`svc *service.Service`; `scheduler/retry.go:110` uses `service.AgentListFilter`;
`scheduler/executor_resolver.go:11` and `scheduler/run.go:54` alias
`service.ExecutorConfig` and `service.LaunchContext`). Making the facade delegate
to `scheduler` would invert an existing edge and create a cycle. So here the
**facade is the owner** and `scheduler`'s copies collapse onto it.

Split across two tasks for a specific reason:

- **Task 09 — run-claim/guard fork.** `ClaimNextRun`, `ProcessRunGuard`,
  `guardAgentStatus`. Mechanical; no cancel-path involvement.
- **Task 10 — retry fork. Blocked.** `scheduleRetry`'s real drift (D4) is
  precisely its `isRetryStale` → `cancelRetry` branch, and `escalateFailure`
  calls `FailRun`. Sibling task **`db3adcf4` is consolidating the run-cancel
  writers**. Task 10 must not start until that lands; see Constraints.

### 3. Drifted pairs — recorded in full

All 37 same-name pairs are diffed and classified in
[`inventory.md`](inventory.md) §"Section B", with the six real differences
recorded as D1–D6 (both behaviors, which side wins, and why). Two are flagged
for human review rather than decided: the D1/D2 interaction (task 03) and D6's
wire-visible event source string (task 07).

### 4. Existing seams — the cheap wins

- `service/skill_compat.go` already re-exports from `skills` — the skills seam
  exists and is used. Task 06 is small for this reason.
- `service/config_read.go:12,30,53,83` are bare one-line wrappers over
  `repo.ListX`. Deleting them is a rename, not a behavior change (task 02).
- `agents/service.go:207,213` already forward `GetAgentInstance` →
  `GetAgentFromConfig` and `ListAgentInstances` → `ListAgentsFromConfig`, so the
  three scheduler call sites in task 05 repoint onto methods that already exist
  with matching signatures.
- `shared.ActivityLoggerImpl` already implements the activity-logging interface
  the facade's `activity.go` re-implements (task 08).

---

## Duplication ledger

Each task's acceptance is a **delta** against its branch point, not an absolute
count — wave-2 tasks may land in any order. The arithmetic must close:

| Task | Section A groups removed | Section B same-name pairs removed |
| --- | ---: | ---: |
| 02 config helpers | 14 | 10 |
| 03 config import drift | 0 | 8 |
| 04 projects | 4 | 2 |
| 05 agents | 7 | 3 |
| 06 skills + routines | 6 | 3 |
| 07 channels + comments + instructions | 3 | 2 |
| 08 activity + onboarding | 3 | 1 |
| 09 scheduler run-claim | 1 | 2 |
| 10 scheduler retry (blocked) | 1 | 3 |
| **Total removed** | **39** | **34** |
| Baseline (2026-08-08) | 40 | 37 |
| **Expected residue** | **1** | **3** |

The residue is out of scope by design and is recorded so it is not mistaken for
incomplete work:

- Section A: `taskIDFromPayload` ↔ `extractRunTaskID`
  (`dashboard/run_detail.go:83` ↔ `scheduler/dispatch_routing.go:636`) — a real
  identical pair, but it involves **no `office/service` copy**. Out of this
  plan's remit; noted for a future task.
- Section B: `publishCommentCreated` ↔ `dashboard` (0.653), `CreateComment` ↔
  `dashboard` (0.646), and `service.Start` ↔ `infra.Start` (0.610). The first two
  are a `dashboard` third copy of the comment path; the third is a coincidental
  Go-boilerplate collision at the detector's floor.

`CreateDefaultInstructions` is an easy double-count: it is an **agents**-domain
group that physically lives in `service/instructions.go`, so it is removed by
task 07, not task 05.

---

## Backend

Frontend section omitted deliberately: this refactor touches no HTTP route, no
response shape, and no user-visible behavior. `office/routes.go` is unchanged by
every task except 11.

### Deletion targets in `internal/office/service`

| File | LOC | Task | Fate |
| --- | --- | --- | --- |
| `config_export.go` `config_sync.go` `config_sync_helpers.go` `config_sync_util.go` `config_read.go` | 840 | 02 | delete; `office/config` owns |
| `config_import.go` | 319 | 03 | delete; `office/config` owns, after D1+D2 reconciliation |
| `service.go` (project CRUD, lines 520-620) | ~100 | 04 | delete; `office/projects` owns |
| `agents.go` | 313 | 05 | delete; `office/agents` owns |
| `service.go` (skill + routine CRUD, lines 461-518, 687-730) | ~100 | 06 | delete; `office/skills`, `office/routines` own |
| `channels.go` `comments.go` `instructions.go` | 247 | 07 | delete; `office/channels`, `office/agents` own |
| `activity.go` | 146 | 08 | delete; `shared.ActivityLoggerImpl` owns |
| `service.go` (`generateSlug`, `writeWorkspaceConfig`, lines 833-869) | ~37 | 08 | delete; `office/onboarding` owns |
| `scheduler_runs.go` | 113 | 09 | delete; `office/scheduler` owns |
| `retry.go` `retry_cancel.go` | 177+ | 10 | **blocked on `db3adcf4`** |

Approximately 2,390 LOC of production code removed across tasks 02–09, plus the
corresponding test files relocated or retired.

### What `office/service` keeps

The run-execution engine: `run.go`, `engine_dispatcher.go`, `prompt_builder.go`,
`env_builder.go`, `wake_payload.go`, `executor_resolver.go`,
`event_subscribers.go`, `failure.go`, `idle_timeout.go`,
`continuation_summary.go`, `task_assignee.go`, `channel_relay.go`,
`skill_manifest.go`, `skill_lookup.go`, `workspace_deletion.go`,
`tree_controls.go`, `scheduler_integration.go`, `scheduler_recovery.go`,
`scheduler_staleness.go`, `pricing.go`, `service.go` (constructor + wiring).

Task 11 records that boundary in `apps/backend/AGENTS.md` so the mirror cannot
grow back.

---

## Tests

The test asymmetry is the single largest risk in this plan and is uneven across
domains. Coverage counted from the working tree:

| Domain | Facade tests | Sub-package tests | Consequence |
| --- | --- | --- | --- |
| config | 4 files, 576 LOC (`config_read_test.go` `config_sync_test.go` `config_test.go` `config_write_test.go`) | **0 files** | Tests **must** move to `office/config` or all config coverage is lost. Tasks 02, 03. |
| projects | `service_project_test.go`, 144 LOC | **0 files** | Must move. Task 04. |
| channels | `channels_test.go` 114 LOC, `channel_relay_test.go` | `handler_test.go` only | Service-layer tests must move. Task 07. |
| agents | `agents_test.go`, 249 LOC | 7 files, 1,240 LOC | Sub-package suite survives; port only uncovered cases. Task 05. |
| skills | none | 8 files, 2,169 LOC | Sub-package suite survives as-is. Task 06. |
| routines | none | 2 files, 534 LOC | Sub-package suite survives as-is. Task 06. |
| scheduler | `retry_test.go` and run tests | 3 files, 848 LOC | Facade is the owner here; scheduler-side tests move *to* the facade. Tasks 09, 10. |

**Rule for every task:** tests move with the code they cover. Where both sides
cover the same function, the surviving suite is the one on the **owning** side,
and the task file names any assertion present only in the retired suite and
ports it. No task may reduce the assertion count for a domain.

Two specific coverage hazards:

1. **Task 03 cannot be a test move.** The facade's config tests encode the
   *unscoped* behavior (D1). Moving them to `office/config` verbatim would make
   them fail, and "fixing" them to pass would re-introduce the bug. Task 03
   writes new workspace-scoping tests first (RED), then reconciles.
2. **`office/config` and `office/projects` have zero tests today.** Tasks 02, 03
   and 04 are the only opportunity to land that coverage; each is required to.

---

## Constraints carried into every task

- **`office/repository/sqlite.Repository` embeds `*runssqlite.Repository`**
  (`office/repository/sqlite/base.go:60`). No task moves ownership of run writes.
  Tasks 02–08 touch no run-write path at all.
- **Sibling task `db3adcf4` is consolidating the run-cancel writers.** The cancel
  path is **left alone**. Task 10 (`retry.go`/`retry_cancel.go`, whose real drift
  D4 *is* the `cancelRetry` branch) is blocked on `db3adcf4` landing and must
  rebase onto it. Tasks 01–09 and 11 are independent of it.
- **Go limits** (`apps/backend/.golangci.yml`): functions ≤80 lines / ≤50
  statements, cyclomatic ≤15, cognitive ≤30, nesting ≤5. Relevant to task 03,
  where merging D1's scoping with D2's validated create path grows `apply*`.
- **No production code change lands without its own green full suite.** Each task
  is a separately mergeable PR.

### Validation, every task

```bash
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && golangci-lint run ./... --new-from-rev=main --timeout=5m
```

Plus the task's own targeted `go test` run, and a detector re-run:

```bash
cd docs/plans/office-service-collapse/officedup && GOTOOLCHAIN=local go run . \
  ../../../../apps/backend/internal/office | head -3
```

**Environment note:** the backend full suite fails ~4 packages in a task worktree
with `parent directory cannot be accessed`. That is the sandbox, not breakage —
CI passes the same commit. Do not treat it as a blocker or try to fix it.

---

## Architecture decision

**Yes, an ADR is warranted** — drafted at
[`docs/decisions/2026-08-08-office-domain-ownership-boundary.md`](../../decisions/2026-08-08-office-domain-ownership-boundary.md).
It meets `/record decision`'s criteria: it establishes a durable ownership
boundary for the office domain (sub-packages own domain CRUD and HTTP;
`office/service` owns run execution) and there are two real, rejected
alternatives — absorb the sub-packages back into the facade, or keep the facade
and make it delegate.

---

## Verification Results

Pending. On completion, synchronize with each task's `## Results`: exact
commands, outcomes/counts, the detector's group count before and after, and the
final `office/service` LOC.

---

## Implementation Waves And Parallel Candidates

Sequenced by risk: tooling first, then identical bodies, then drifted bodies,
then the scheduler fork. Leaf domains (no facade consumer) precede domains the
facade's own callers depend on (agents, reached by `scheduler`).

```
Wave 1 (tooling — no production code):
- [ ] [task-01-duplication-detector](task-01-duplication-detector.md)

Wave 2 (identical bodies, leaf domains — parallel candidates, disjoint files):
- [ ] [task-02-config-helpers](task-02-config-helpers.md)
- [ ] [task-04-projects-domain](task-04-projects-domain.md)
- [ ] [task-06-skills-routines](task-06-skills-routines.md)

Wave 3 (drifted bodies, and domains with facade consumers):
- [ ] [task-03-config-import-drift](task-03-config-import-drift.md)   (depends on 02)
- [ ] [task-05-agents-domain](task-05-agents-domain.md)
- [ ] [task-07-channels-comments](task-07-channels-comments.md)
- [ ] [task-08-activity-onboarding](task-08-activity-onboarding.md)

Wave 4 (scheduler fork):
- [ ] [task-09-scheduler-run-claim](task-09-scheduler-run-claim.md)
- [ ] [task-10-scheduler-retry](task-10-scheduler-retry.md)   BLOCKED on db3adcf4

Wave 5 (close out):
- [ ] [task-11-narrow-facade-and-document](task-11-narrow-facade-and-document.md)
```

Waves 2 and 3 list parallel *candidates* only. Default execution is sequential in
the primary conversation; waves do not authorize subagents.

## Open Questions

- **D6 (task 07):** changing the `events.OfficeCommentCreated` source string from
  `"office-service"` to `"channels-service"` is wire-visible. No in-repo consumer
  filters on it, but a plugin could. Task 07 asks for a decision rather than
  assuming.
- **Task 10 rebase surface:** the exact overlap with `db3adcf4` is unknown until
  that task lands. Task 10's scope is provisional.
