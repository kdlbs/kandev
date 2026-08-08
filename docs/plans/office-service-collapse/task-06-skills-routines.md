---
id: "06-skills-routines"
title: "Delete the facade's skills and routines mirrors"
status: pending
wave: 2
depends_on: ["01-duplication-detector"]
plan: "plan.md"
spec: none
parallel-safe: true
---

# Task 06: Skills and Routines Domains

The cheapest task in the plan: both sub-packages are well tested, both own their
routes, and the skills seam **already exists** — `service/skill_compat.go`
re-exports `skills.AgentTypeResolver`, `skills.ProjectSkillDirResolver`,
`skills.ParseDesiredSlugs`, `SkillSourceTypeInline` and `DefaultProjectSkillDir`.
The facade already depends on `office/skills`; it just never deleted its own copy
of the CRUD.

## Scope

Delete from `internal/office/service/service.go`:

- skills (lines ~461–518): `CreateSkill` `UpdateSkill` `DeleteSkill`
  `prepareServiceSkillPackageMetadata`
- routines (lines ~687–730): `CreateRoutine` `UpdateRoutine` `DeleteRoutine`
  `GetRoutine` `ListRoutines`

and `GetSkillFromConfig` / `ListSkillsFromConfig` / `GetRoutineFromConfig` /
`ListRoutinesFromConfig` from `config_read.go` if task 02 left them.

Identical groups: `DeleteSkill`, `prepareServiceSkillPackageMetadata` →
`prepareSkillPackageMetadata` (**renamed**), `CreateRoutine`, `UpdateRoutine`,
`DeleteRoutine`, `GetRoutineFromConfig`.

`CreateSkill` (0.972) and `UpdateSkill` (0.952) differ only by that helper
rename. `applyRoutines`' use of `CreateRoutine` is a bare repo call plus an error
wrap — there is no validation to preserve on the routines side.

### D5 — sentinel error, decided

`GetSkillFromConfig`: the facade returns `fmt.Errorf("skill not found: %s", …)`;
`skills/service.go:237` returns `fmt.Errorf("%w: %s", ErrSkillNotFound, …)`.
**`office/skills` wins** — only its form is matchable with `errors.Is`.

One caller reaches this through an interface:
`internal/agent/runtime/lifecycle/skill/manifest.go:45`
(`d.skillReader.GetSkillFromConfig`). Confirm that call site's error handling
before switching — if it string-matches `"skill not found"`, update it to
`errors.Is(err, skills.ErrSkillNotFound)` in the same commit.
`skills/errors_test.go` already exists; extend it rather than adding a new file.

## Test migration

None required. `office/skills` has 8 test files / 2,169 LOC and
`office/routines` 2 files / 534 LOC; the facade has **no** skill or routine test
files. Nothing is lost. Add only the D5 caller test described above.

## Acceptance

1. Detector Section A drops by **6** groups; Section B same-name pairs drop by 3.
2. `errors.Is(err, skills.ErrSkillNotFound)` holds at the
   `lifecycle/skill/manifest.go` call site.
3. `make -C apps/backend test` green.

## Verification

```bash
cd apps/backend && go test ./internal/office/skills/... ./internal/office/routines/... -count=1
cd apps/backend && go test ./internal/agent/runtime/lifecycle/skill/... -count=1 -v
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && golangci-lint run ./... --new-from-rev=main --timeout=5m
```

## Files likely touched

- `internal/office/service/service.go` (delete the skill and routine blocks)
- `internal/office/service/config_read.go`
- possibly `internal/office/service/skill_compat.go` (drop now-unused re-exports)
- `internal/agent/runtime/lifecycle/skill/manifest.go` (D5 error handling)
- `internal/office/skills/errors_test.go`

## Dependencies

Task 01.

## Parallelism

`parallel-safe` with tasks 02 and 04 in principle, but all three edit
`internal/office/service/service.go` — expect a rebase if run concurrently.

## Rollback position

Single revert. If only the D5 half needs backing out, the sentinel-error change
at the `manifest.go` call site is independently revertible from the deletions.

## Output contract

Summary, files changed, detector delta, and the `manifest.go` error-handling
decision with evidence of how that call site previously matched the error.

## Results

Pending.
