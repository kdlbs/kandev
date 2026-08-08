---
id: "03-config-import-drift"
title: "Reconcile the config import/preview drift (D1 + D2)"
status: pending
wave: 3
depends_on: ["02-config-helpers"]
plan: "plan.md"
spec: none
parallel-safe: false
---

# Task 03: Config Import / Preview Drift

**This is the highest-risk task in the plan.** It is the one place where a
mechanical "delete one copy" would silently pick a behavior, and where *neither*
copy is correct on its own.

## The drift

Eight pairs: `preview{Agents,Projects,Skills,Routines}` and
`apply{Agents,Projects,Skills,Routines}`, between
`service/config_import.go` and `config/import.go`.

**D1 — workspace scoping. `office/config` is correct.**

The facade declares the workspace parameter as `_ string` and lists with
`ListXFromConfig(ctx, "")` — *every workspace*. `office/config` takes `wsID` and
lists `repo.ListX(ctx, wsID)`. On the create branch `office/config` sets
`WorkspaceID: wsID` on the new row; **the facade sets no `WorkspaceID` at all**.

Consequences of the facade's version: importing a bundle into workspace B can
match an entry by name against a row in workspace A and update it; and rows it
creates have an empty `WorkspaceID`. This is not defensible as intentional.

**D2 — validated create path. The facade is correct.**

The facade calls `s.CreateProject` / `s.CreateSkill` / `s.CreateAgentInstance` /
`s.UpdateProject`. `office/config` calls `s.repo.CreateX` **directly**, bypassing:

- `service/service.go:520` `CreateProject` — `validateProject`, then defaults
  `Status`→`ProjectStatusActive`, `Repositories`→`"[]"`, `ExecutorConfig`→`"{}"`
- `service/agents.go:103` `CreateAgentInstance` — `validateAgentCreate`, then
  defaults `ID` (uuid), `Permissions`→`DefaultPermissions(role)`,
  `MaxConcurrentSessions`→1, `CooldownSec`→10
- `skills.CreateSkill` — defaults `SourceType`→inline, `FileInventory`→`"[]"`,
  runs `prepareSkillPackageMetadata`

So `office/config` currently writes rows that skip validation and defaulting.

**These two point in opposite directions and must be taken together.** The
correct end state is `office/config`'s workspace scoping **plus** the validated
create path, which today exists on neither side.

## Approach (TDD — RED first)

`office/config` has zero tests, so there is nothing to protect the reconciliation.
Write the tests before touching the code.

1. **RED — workspace scoping.** In `config/import_test.go`: two workspaces each
   holding a same-named agent/project/skill/routine; import a bundle into
   workspace B; assert workspace A's row is untouched and the created/updated row
   carries `WorkspaceID == B`. This passes on `office/config` today and fails on
   the facade — it pins D1.
2. **RED — validation and defaulting.** Import a bundle whose entries omit
   `Status` / `Repositories` / `Permissions` / `SourceType`; assert the persisted
   rows carry the documented defaults, and that an entry violating
   `validateProject` / `validateAgentCreate` is rejected. **This fails on
   `office/config` today** — it is the regression D2 describes.
3. **GREEN.** In `office/config`, replace the direct `repo.CreateX`/`UpdateX`
   calls with the owning sub-package's validated methods
   (`projects.ProjectService.CreateProject`, `agents.AgentService.CreateAgentInstance`,
   `skills.SkillService.CreateSkill`, `routines.RoutineService.CreateRoutine`),
   injected as narrow interfaces on `ConfigService`. Keep the `wsID` scoping.
4. Delete `service/config_import.go` and migrate the import/preview cases held
   back from `service/config_test.go` by task 02.

> `routines.CreateRoutine` is a bare repo call plus an error wrap
> (`service/service.go:687`), so for routines steps 3's change is a no-op beyond
> setting `WorkspaceID`. Do not invent validation that does not exist.

## Watch the Go limits

Merging scoping with the validated create path grows the `apply*` functions,
which are already 36–43 lines. `.golangci.yml` caps functions at 80 lines / 50
statements, cyclomatic 15, cognitive 30, nesting 5. Extract a per-entity
`upsertEntry` helper rather than growing the four `apply*` bodies in parallel.

## Acceptance

1. Detector Section A is **unchanged** (these eight are Section B pairs, not
   Section A); Section B same-name pairs drop by **8**
   (`preview{Agents,Projects,Skills,Routines}`,
   `apply{Agents,Projects,Skills,Routines}`). Measure as a delta against the
   branch point; see the ledger in [`plan.md`](plan.md).
2. Importing into workspace B never reads or writes a row in workspace A.
3. Imported rows carry `WorkspaceID` and the documented defaults; invalid
   entries are rejected.
4. `internal/office/service/config_import.go` no longer exists.

## Verification

```bash
cd apps/backend && go test ./internal/office/config/... -run 'TestImport|TestPreview|TestApply' -count=1 -v
cd apps/backend && go test ./internal/office/... -count=1
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && golangci-lint run ./... --new-from-rev=main --timeout=5m
```

## Files likely touched

- `internal/office/config/import.go`, `internal/office/config/service.go`
  (new narrow interface fields)
- `internal/office/config/import_test.go` (new)
- deleted: `internal/office/service/config_import.go`
- `internal/office/services.go` and/or `internal/backendapp/main.go` — wiring the
  validated collaborators into `config.ConfigService`
- migrated import/preview cases from `internal/office/service/config_test.go`

## Dependencies

Task 02 (owns the rest of the config surface and the test relocation).

## Parallelism

`sequential`. Touches `office/services.go` wiring, which several other tasks read.

## Rollback position

Revert to task 02's state: `office/config` keeps the unvalidated-but-scoped
behavior it has today, and the facade's `config_import.go` is already gone. **The
scoping fix (D1) and the validation fix (D2) must land in the same commit** —
splitting them leaves a window where imports are both unscoped and unvalidated.

## Escalation

If step 3 cannot inject the validated methods without an import cycle
(`config` → `projects`/`agents`/`skills` while any of those imports `config`),
**stop and report** rather than reaching for a repo-direct shortcut. That
cycle would be a genuine finding about the boundary this plan's ADR establishes.

## Output contract

Summary, files changed, both RED tests quoted with their pre-fix failure output,
Section B pair delta 37→29, and confirmation that D1 and D2 landed together.

## Results

Pending.
