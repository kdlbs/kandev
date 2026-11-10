---
id: "01-compare-directory-identity"
title: "Compare recorded sources by directory identity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
acceptance_criteria:
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.5
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.6
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
---

# Task 01: Compare recorded sources by directory identity

## Summary

Make managed clone admission accept populated persisted source paths that
identify the same physical directories as Git registration. Prove ordinary
executor preflight preserves the task's checkout and conversation while keeping
true identity mismatches blocked.

## In scope

- Add failing tests before correcting `verifyRecordedManagedCloneSource`.
- Use the existing `sameDirectoryIdentity` helper for both recorded directories
  after canonical resolution; preserve empty, partial, and invalid evidence rules.
- Add real-Git manager and SQLite-backed executor regression coverage.
- Record actual verification results in this file and the plan.

## Out of scope

- Other path predicates without independent reproduction.
- Changes to origins, ownership, claims, slot selection, runtime admission,
  recovery actions, or provider tokens.
- Live task/database/Git metadata changes, publication, and delegation.

## Acceptance

1. With populated recorded fields, aliases of either source path, common
   directory, or both are accepted only when the directories are physically
   identical. Cover GitHub/GitLab, owner/provider layout, and linked/main checkout
   through the existing fixture, with filesystem-specific skips narrowly scoped.
2. Two empty fields keep their legacy behavior; one empty field, a missing path,
   or a different physical clone/common directory remains an error. Always run
   portable negatives. On a case-sensitive filesystem, two different directories
   whose spellings differ only in case must remain distinct. A healthy alias
   slot plus an invalid sibling refuses admission without checkout or inventory
   changes.
3. Real executor preflight consumes persisted case-alias source evidence and
   preserves branch, HEAD, index, file contents, complete environment inventory,
   session state, and provider conversation metadata. Reuse does not create a
   destination clone or relocation operation.

## Verification

Run from the repository root. First run the new test names against unchanged
production code and record the expected identity refusal on a case-insensitive
filesystem. Then implement the minimal correction and run:

```bash
export TMPDIR="$(python3 -c 'import os,tempfile; print(os.path.realpath(tempfile.gettempdir()))')"
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree ./internal/orchestrator/executor -run 'RecordedManagedCloneSourceIdentity|RecordedSourceCaseAlias|RecordedSourceAliasWithInvalidSibling|LegacyClone|LegacyReuse|RegisteredLegacy' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -run 'ManagedClone|ManagedMain|Relocat|ParseManagedGitOrigin' -count=1)
bash scripts/lint-go-changed apps/backend/internal/worktree/managed_clone_relocation.go apps/backend/internal/worktree/managed_clone_recorded_source_identity_test.go apps/backend/internal/orchestrator/executor/executor_legacy_clone_recovery_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If a baseline platform failure occurs, distinguish it from the new regression
using a focused baseline run; record the limitation without reporting the whole
command as passed. Do not broaden into unrelated full suites.

## Files likely touched

- `apps/backend/internal/worktree/managed_clone_relocation.go`
- `apps/backend/internal/worktree/managed_clone_recorded_source_identity_test.go`
- `apps/backend/internal/orchestrator/executor/executor_legacy_clone_recovery_test.go`
- `docs/plans/recorded-source-path-identity/plan.md`
- `docs/plans/recorded-source-path-identity/task-01-compare-directory-identity.md`

## Dependencies

None. Execute this single work order after the explicit implementation request.

## Risks

- Global case folding weakens identity checks on case-sensitive hosts.
- The existing case-alias test omits recorded evidence; it cannot prove this fix.
- Do not use mutable live workspaces or the user's Git configuration as fixtures.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/managed-clone-relocation.md),
  AC-TASKS-MANAGED-CLONE-RELOCATION-001.5 and .001.6.
- [System design](../../specs/tasks/system-design/managed-clone-relocation.md),
  unchanged legacy source admission and all-selected-slot validation.
- [Relocation boundary ADR](../../decisions/2026-09-27-managed-clone-relocation-boundary.md).
- Existing `newLegacyReuseFixture`, legacy alias/refusal tests,
  `sameDirectoryIdentity`, and SQLite executor preflight fixture.
- Root and backend `AGENTS.md`; `/tdd` backend testing guidance.

## Results

Completed on October 22, 2026. The regressions failed before the relevant
production correction and passed afterward. Final combined Worktree/executor
verification covered both work orders, including real Git and SQLite-backed
preflight/reuse with unchanged inventory, HEAD, file bytes, and provider metadata.

- Casing and retargeting/selection suites: passed (Worktree 19.052s,
  executor 11.861s).
- Managed clone/relocation compatibility suite: passed (50.540s) with
  canonical `TMPDIR=/private/tmp` on macOS.
- Manager required-reuse suite: passed (5.017s).
- Catalog/specification lint, 36 specification-linter tests, 63 public-doc tests,
  54 public-page validations, documentation coverage preflight, and diff checks
  passed. Changed-package Go lint is recorded in the plan after its final run.

Default macOS temporary paths exposed existing `/var` versus `/private/var`
fixture failures. Three representative failures reproduced with an overlay
restoring the original production file. Resolving temporary-path aliases made
the full compatibility scope pass; no production workaround was added for those
fixtures. No live task, database, Git metadata, runtime, or provider token changed.
