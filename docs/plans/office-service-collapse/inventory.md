# Duplication Inventory — `internal/office/service` vs office sub-packages

Regenerated 2026-08-08 against `fb1d8fdcd`. This file is the evidence base for
`plan.md`. Re-run the detector before starting any task and reconcile.

## How this was regenerated

The inventory in the original task description was produced with normalized-body
text hashing (comments/whitespace stripped, ≥6 lines, ≥120 chars) plus Jaccard
token similarity. That heuristic is name-keyed and text-based, so it misses
copies whose receiver identifier differs and copies that were renamed.

This inventory was regenerated with an **AST-based** detector committed beside
this file at `officedup/`:

```bash
cd docs/plans/office-service-collapse/officedup
GOTOOLCHAIN=local go run . ../../../../apps/backend/internal/office
```

Method, and why each step matters:

1. Parse every non-test `.go` file under `internal/office/**` with `go/parser`
   in mode `0`, which **drops comments entirely** — so a reworded doc comment
   cannot register as drift (this is what makes `SetupChannel` read as a copy).
2. Rewrite every use of the **receiver identifier** to the fixed token `RCV`
   before printing. This is the step the text heuristic lacks: it makes
   `func (s *Service)` and `func (ss *SchedulerService)` copies hash identically,
   and it is what surfaced the 8 extra groups below.
3. Re-print each body with `go/printer` in `RawFormat` and collapse whitespace.
   Formatting and line-wrapping differences vanish.
4. **Section A** = SHA-256 of that normalized body, grouped; report any group
   spanning more than one package. Same floor as the original heuristic
   (≥6 lines, ≥120 chars) so the two are comparable.
5. **Section B** = Jaccard similarity over the `go/scanner` token *multiset*
   (`sum(min)/sum(max)`) between every `service` function and every
   non-`service` function, reported at ≥0.60.

Known limits, stated so the next person does not over-trust this file:

- It compares **bodies only**. Two functions with identical bodies but different
  signatures (parameter order, return types) are reported as identical.
- Identifier renames *other than the receiver* still count as drift, which is
  why several cosmetic pairs land at 0.95–0.99 rather than 1.00.
- Section B is quadratic over ~1,486 functions; it reports 303 pairs at ≥0.60.
  Only the 37 **same-name** pairs are meaningful; the rest are Go boilerplate
  collisions (guard clause + repo call + error wrap) and were discarded.

## Reconciliation with the original inventory

All 32 groups from the original list reproduce exactly (33 names, but
`prepareServiceSkillPackageMetadata` / `prepareSkillPackageMetadata` are one
renamed pair). The AST detector finds **40** groups — a strict superset. The 8
additional groups, all missed because the text heuristic keys on name and does
not normalize the receiver:

| Group | Locations | Why it was missed |
| --- | --- | --- |
| `ClaimNextRun` | `scheduler/run_processing.go:15` ↔ `service/scheduler_runs.go:18` | receiver `ss` vs `s` |
| `CreateRoutine` | `routines/service.go:240` ↔ `service/service.go:687` | receiver `s *RoutineService` vs `s *Service` |
| `UpdateRoutine` | `routines/service.go:258` ↔ `service/service.go:705` | same |
| `ListActivityFiltered` | `dashboard/service.go:600` ↔ `service/activity.go:124` | different line spans (6L vs 11L) |
| `countAgentsByRoleInWorkspace` ↔ `countAgentsByRole` | `agents/service.go:793` ↔ `service/agents.go:266` | **renamed** |
| `generateSlug` | `onboarding/service.go:506` ↔ `service/service.go:833` | receiver |
| `writeWorkspaceConfig` | `onboarding/service.go:479` ↔ `service/service.go:848` | receiver |
| `taskIDFromPayload` ↔ `extractRunTaskID` | `dashboard/run_detail.go:83` ↔ `scheduler/dispatch_routing.go:636` | **renamed**, and involves no `service` copy at all |

Two consequences for the plan: `office/onboarding` and `office/dashboard` are
also forks of the facade (neither was in the original scope), and `office/scheduler`
is confirmed as a third fork (see plan §Scheduler).

## Section A — identical bodies, by collapse domain (40 groups)

| Domain | Groups | Names |
| --- | --- | --- |
| config | 14 | `ApplyIncoming` `ApplyOutgoing` `ExportBundle` `IncomingDiff` `OutgoingDiff` `PreviewImport` `ScanFilesystem` `bundleToZip` `deleteRowsMissingFromBundle` `diffBundles` `missingNames` `parseErrorsFromLoader` `writeBundleEntities` `writeYAMLFile` |
| agents | 8 | `CreateDefaultInstructions` `GetAgentFromConfig` `ListAgentInstancesFiltered` `UpdateAgentInstance` `UpdateAgentStatus` `validateReportsTo` `validateStatusTransition` `countAgentsByRole`→`countAgentsByRoleInWorkspace` |
| projects | 4 | `DeleteProject` `GetProjectFromConfig` `UpdateProject` `validateRepositories` |
| routines | 4 | `CreateRoutine` `UpdateRoutine` `DeleteRoutine` `GetRoutineFromConfig` |
| skills | 2 | `DeleteSkill` `prepareServiceSkillPackageMetadata`→`prepareSkillPackageMetadata` |
| channels | 2 | `CreateComment` `HandleChannelInbound` |
| scheduler | 2 | `ClaimNextRun` `retryDelayWithJitter` |
| onboarding | 2 | `generateSlug` `writeWorkspaceConfig` |
| dashboard | 1 | `ListActivityFiltered` |
| dashboard ↔ scheduler | 1 | `taskIDFromPayload`→`extractRunTaskID` (no `service` copy; out of scope) |

## Section B — the 37 same-name near-duplicate pairs

The headline result: **31 of the 37 differ only cosmetically.** The original
description flagged 4 pairs as "already drifted — where the risk is"
(`ApplyImport` 0.98, `CreateProject` 0.97, `validateProject` 0.97,
`applyAgents` 0.95). Similarity score does not track behavioral risk here:
`ApplyImport` at 0.990 is cosmetic, while `validateAgentNameUnique` at 0.845 and
`scheduleRetry` at 0.745 are the two most consequential differences in the tree.

Cosmetic difference classes, all verified against the source:

1. **Receiver type rename** — `*Service` → `*ConfigService` / `*AgentService` /
   `*ProjectService` / `*SkillService` / `*ChannelService` / `*SchedulerService`.
2. **Type alias** — `projects/models.go:7` declares `type Project = models.Project`
   and `projects/models.go:15-18,28` alias the status constants. So
   `models.Project` vs `Project` and `models.ValidProjectStatuses` vs
   `ValidProjectStatuses` are *the same identifiers*. This alone accounts for
   `CreateProject` (0.985) and `validateProject` (0.974).
3. **One-line wrapper vs its target** — `service/config_read.go:12,30,53,83`
   define `ListAgentsFromConfig` / `ListSkillsFromConfig` /
   `ListProjectsFromConfig` / `ListRoutinesFromConfig` as bare `return
   s.repo.ListX(ctx, workspaceID)`. Every `export*` diff is this substitution.
   Likewise `service/channels.go:90` `createChannelTask` is a pass-through to
   `repo.CreateChannelTask`, and `agents/service.go:207` `GetAgentInstance` is a
   pass-through to `GetAgentFromConfig`. So `SetupChannel` (0.975) is cosmetic.
4. **Collaborator indirection** — `s.LogActivity(...)` vs
   `s.activity.LogActivity(...)`; `s.GetAgentFromConfig` vs
   `ss.svc.GetAgentFromConfig`. Same target, different hop.
5. **Parameter rename** — `workspaceID` vs `wsID` (`LogActivityWithRun`, 0.982).
6. **Doc-comment rewording** — invisible to the AST detector, visible to `diff`.

### The 6 differences that are real

Ordered by consequence, not by similarity score.

| # | Pair | Difference | Which side is correct |
| --- | --- | --- | --- |
| D1 | `preview{Agents,Projects,Skills,Routines}`, `apply{Agents,Projects,Skills,Routines}` — `service/config_import.go` ↔ `config/import.go` | The facade declares the workspace parameter as `_ string` and lists with `ListXFromConfig(ctx, "")` — **every workspace**. `config` takes `wsID` and lists `repo.ListX(ctx, wsID)`. On the create branch, `config` sets `WorkspaceID: wsID` on the new row; **the facade sets no `WorkspaceID` at all**. | **`config`.** The facade's version matches bundle entries by name across all workspaces, so importing into workspace B can update a row in workspace A, and rows it creates have an empty `WorkspaceID`. Not defensible as intentional. |
| D2 | Same 4 `apply*` pairs, create/update branch | Facade calls the validated service methods `CreateProject` / `CreateSkill` / `CreateAgentInstance` / `UpdateProject`…; `config` calls `s.repo.CreateX` **directly**, bypassing them. `service/service.go:520` `CreateProject` validates and defaults `Status`, `Repositories`→`"[]"`, `ExecutorConfig`→`"{}"`; `service/agents.go:103` `CreateAgentInstance` validates and defaults `ID`, `Permissions`, `MaxConcurrentSessions`, `CooldownSec`; `skills.CreateSkill` defaults `SourceType`, `FileInventory` and runs `prepareSkillPackageMetadata`. | **The facade.** `config` silently writes rows that skip validation and defaulting. Note this is the *opposite* direction from D1 — the correct collapse is `config`'s scoping **plus** the facade's validated create path. Neither side is wholly right. |
| D3 | `validateAgentNameUnique` — `service/agents.go:275` ↔ `agents/service.go:804` (0.845) | On a repository error the facade does `return nil`; `agents` does `return fmt.Errorf("check agent name uniqueness: %w", err)`. | **`agents`.** The facade swallows the error and admits a duplicate agent name whenever the uniqueness query fails. |
| D4 | `scheduleRetry` — `service/retry.go:46` ↔ `scheduler/retry.go:50` (0.745) | The facade opens with `if stale, reason := isRetryStale(run); stale { return s.cancelRetry(ctx, run, reason) }` and logs `source=backoff`. `scheduler` has neither. | **The facade.** `scheduler` retries runs the facade would cancel as stale. **This pair touches `cancelRetry` — see the `db3adcf4` dependency in `plan.md`; it is deferred to task 10.** |
| D5 | `GetSkillFromConfig` — `service/config_read.go:35` ↔ `skills/service.go:237` (0.958) | Facade returns `fmt.Errorf("skill not found: %s", …)`; `skills` returns `fmt.Errorf("%w: %s", ErrSkillNotFound, …)`. | **`skills`.** Only its form is matchable with `errors.Is`. `internal/agent/runtime/lifecycle/skill/manifest.go:45` calls this through an interface — confirm its error handling before switching. |
| D6 | `publishCommentCreated` — `service/comments.go:41` ↔ `channels/comments.go:34` (0.960) | Event source string `"office-service"` vs `"channels-service"`; payload type `CommentPostedData` (exported) vs `commentPostedData` (unexported). | **`channels`**, but the source string is **observable on the event bus**. Flagged for review — see task 07. |

### Flagged for review, not decided here

- **D2 vs D1 interact.** Adopting `config`'s repo-direct writes to get workspace
  scoping would *also* adopt the validation bypass. Task 03 must take both
  halves or neither, and it needs new tests, because `office/config` has **zero**
  test files today.
- **D6's event source string.** Changing `"office-service"` → `"channels-service"`
  is a wire-visible change to `events.OfficeCommentCreated`. No in-repo consumer
  filters on it (verified by grep), but it is the kind of thing a plugin could.

## Live consumers of `*service.Service` (the collapse-direction evidence)

`internal/office/service` contains **no `gin` import** — it has no HTTP surface.
`office/routes.go` registers handlers built exclusively from the sub-package
services, and `office/services.go:26-40` wires `Agents`/`Skills`/`Projects`/
`Routines`/`Config`/`Channels`/`Costs`/`Approvals`/`Labels` to sub-package types.
The `office/runtime` action interfaces (`Comments`, `Skills`, `Projects`,
`Agents`, `AgentModifier`) resolve to `svcs.Dashboard` / `svcs.Skills` /
`svcs.Projects` / `svcs.Agents` — **not** to the facade.

The facade's entire live external method surface is 17 methods:

| Consumer | Methods |
| --- | --- |
| `office/tree_controls` | `CancelTaskTree` `PauseTaskTree` `ResumeTaskTree` `RestoreTaskTree` `PreviewTaskTree` `GetSubtreeCostSummary` |
| `office/workspaces` | `DeleteWorkspace` `GetWorkspaceDeletionSummary` |
| `office/scheduler` | `BuildWakePayload` `ResolveExecutor` `GetAgentFromConfig` `ListAgentInstances` `ListAgentInstancesFiltered` `LogActivityWithRun` |
| `internal/backendapp` | `SetWorkflowEngineDispatcher` `ListFailedRunInboxRows` `ListPausedAgentInboxRows` + `Set*`/`Register*` wiring calls |

Every duplicated CRUD/config method in the facade is reachable **only** from
within `service` itself, from its own tests, or from those 6 scheduler calls.

### A note on static-analysis limits

Neither tool settles this on its own, and they fail in **opposite** directions:

- `deadcode -test ./...` reports only 4 unreachable funcs in `office/service`,
  and `deadcode ./...` only 19 — because it is RTA-based: once `*service.Service`
  is converted to any interface, every method whose name matches an
  interface-method invoked anywhere in the program is marked reachable. It
  **over**-approximates through interface dispatch.
- `grep` for `.Method(` **under**-approximates: it cannot see interface dispatch
  at all.

The reliable determination is the composition root — which concrete value is
wired into each interface field in `office/services.go` and
`office/routes.go`. That is the evidence used above, and it is what any
re-verification should re-check.
