---
id: "01-select-available-remote-source"
title: "Select a cloneable source after local deletion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REMOTE-RESOLUTION-001
acceptance_criteria:
  - AC-WORKSPACES-REMOTE-RESOLUTION-001.1
  - AC-WORKSPACES-REMOTE-RESOLUTION-001.2
  - AC-WORKSPACES-REMOTE-RESOLUTION-001.3
  - AC-WORKSPACES-REMOTE-RESOLUTION-001.4
  - AC-WORKSPACES-REMOTE-RESOLUTION-001.5
  - AC-WORKSPACES-REMOTE-RESOLUTION-001.6
system_design:
  - ../../specs/workspaces/system-design/remote-repository-resolution.md
---

# Select a cloneable source after local deletion

## Summary

Repair remote selection at the task service boundary and prove that the selected
registration reaches managed checkout preparation without modifying the
deleted local source. Execute sequentially in the primary session.

## Scope

- Add the failing real-checkout/deletion service regression before implementation.
- Add a private candidate eligibility/selection helper; invoke it from remote
  `FindOrCreateRepository` requests under the existing resolution mutex.
- Preserve current creation, backfill, identity, and rollback contracts.
- Cover every candidate and failure case listed in the plan's test matrix.
- Prove real service resolution, persisted attachment, managed preparation, and
  worktree creation with a local Git origin and fake final agent boundary.

## Exclusions

- Production executor changes, SQL/schema changes, and rendered UI changes.
- Existing-task relocation, credential redesign, and local folder restoration.
- Copying skipped local registration settings or secrets into managed fallback.

## Acceptance conditions

1. The deletion regression fails before the correction and passes afterward;
   the integration reaches a valid managed workspace with the requested branch.
2. Mixed, repeated, and concurrent selections converge on an eligible matching
   source while preserving skipped local rows, links, paths, and contents.
3. Existing explicit-local and provider-clone controls pass; identity and
   inspection failures cannot select a foreign candidate or start an agent in
   the deleted path.

## Verification

Run from the repository root. The first command is the Red-stage check and
must fail for deleted-local adoption before the implementation change.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run '^TestResolveRepositoryRef_RemoteSelectionSkipsDeletedLocalCheckout$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/service ./internal/orchestrator/executor -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record the Red result, final package results, and documentation cross-reference
coverage preflight in Results. Use `validateCoverage` from
`.github/scripts/pr-docs.cjs` against the changed work order, linked plan,
requirement/design contents, and actual changed application paths. Promote the
paired specs only after implementation and all checks pass.

## Files likely touched

- `apps/backend/internal/task/service/service_resources.go`
- `apps/backend/internal/task/service/remote_repository_resolution.go` (new)
- `apps/backend/internal/task/service/remote_repository_resolution_test.go` (new)
- `apps/backend/internal/task/service/remote_repository_admission_test.go` (review regressions)
- `apps/backend/internal/orchestrator/executor/executor_remote_selection_integration_test.go` (new)
- This work order, `plan.md`, and the paired requirement/design lifecycle fields.
- `docs/public/tasks-and-workflows.md` (remote selection recovery guidance).

## Dependencies

None.

## Risks

Use raw workspace rows for fallback, preserving existing SQL identity semantics
and creation ordering. Only a confirmed absent directory qualifies for this
repair. The integration fixture must use owned temporary files, close its
database, and stop every service it starts. No developer runtime or internet
clone is needed. Skip permission assertions when the host bypasses permissions.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/remote-repository-resolution.md)
- [Design](../../specs/workspaces/system-design/remote-repository-resolution.md)
- `service_resources_test.go` and `provider_scope_test.go` for service patterns.
- `executor_resume_clone_default_branch_test.go` for clone/worktree fixtures.
- `.agents/skills/tdd/references/backend-tests.md` for fixture isolation.

## Results

Completed in the primary session without delegated agents.

- Red: the documented deletion-regression command failed because it selected
  the original deleted local registration (`created=false`, `source=local`).
- Green: `go test -trimpath -tags fts5 ./internal/task/service -run
  '^TestResolveRepositoryRef_RemoteSelection' -count=1` passed.
- Final package check: the documented `go test -trimpath -tags fts5 -race
  ./internal/task/service ./internal/orchestrator/executor -count=1` passed
  after the final production refactor (service: 60.026s; executor: 10.871s).
- Additional scope case: `go test -trimpath -tags fts5 -race
  ./internal/task/service -run
  '^TestResolveRepositoryRef_RemoteSelectionScopedFallback$' -count=1 -v` passed.
- `golangci-lint run ./internal/task/service ./internal/orchestrator/executor
  --new-from-rev=HEAD --timeout=5m`: zero issues.
- `python3 scripts/list-docs.py validate`: passed (343 decisions, 1313 specifications).
- `python3 scripts/lint-spec-files.test.py`: all 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: all 62 tests passed.
- `node scripts/validate-public-docs.mjs`: passed (47 published pages).
- `validateCoverage` from `.github/scripts/pr-docs.cjs`, with actual tracked
  and untracked changed paths plus the linked document contents: `covered`.
- `git diff --check`: passed.

Node commands used the installed v24.18.0 binary because Node was absent from
the shell PATH. Public docs retain their task-oriented how-to format. No
production executor, API, schema, or UI changes were necessary.

### Review remediation results

- Red: `go test -trimpath -tags fts5 ./internal/task/service -run
  '^TestResolveRepositoryRef_RemoteSelection(UnscopedAdmission|RejectsChangedOrigin)$'
  -count=1` failed on both findings before the correction.
- Green: all `TestResolveRepositoryRef_RemoteSelection` cases passed.
- Final: `go test -trimpath -tags fts5 -race ./internal/task/service
  ./internal/orchestrator/executor -count=1` passed (65.532s and 11.317s).
- `golangci-lint run ./internal/task/service ./internal/orchestrator/executor
  --new-from-rev=HEAD --timeout=5m` before the remediation commit: zero issues.
- Spec catalog validation, full spec lint, validation of 47 public docs pages,
  and whitespace checks passed after the requirement/design/public-doc updates.

Added `remote_repository_admission_test.go` for initial and fallback scope
isolation, changed origin rejection with row preservation, and compatible
SSH/HTTPS reuse. Local fixtures now carry a matching origin. Remote CI and
review for the remediation remain pending until publication.
