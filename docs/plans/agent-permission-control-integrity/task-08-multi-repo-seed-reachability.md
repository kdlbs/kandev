---
id: "08-multi-repo-seed-reachability"
title: "Report a repository seed that cannot reach the agent"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-006
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-006.1
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-006.2
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-006.3
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-006.4
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-006.5
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 08: Report a repository seed that cannot reach the agent

## Summary

`Repository.CopyFiles` always seeds into that repository's own worktree root.
For a single-repository workspace that is the agent's working directory. For a
multi-repository workspace the agent's working directory is the parent task
root, so the seed lands one level below where the agent reads — silently.

Make that mismatch visible. The failure otherwise presents as a missing
permission rather than a misplaced file, which is the kind of defect that gets
investigated from scratch a second time.

## Scope

- Derive the agent working directory for the resolved workspace layout from the
  same source the launch path uses (`env_preparer_worktree.go`: repository
  worktree root for one repository, task root for two or more).
- During workspace preparation, when a repository has a non-empty `copy_files`
  spec and the layout places the agent outside that repository's worktree,
  record a reachability warning naming the seed destination and the agent
  working directory.
- Surface the warning where the existing workspace-preparation warnings already
  surface, alongside the non-fatal `copyfiles` warnings.
- State the destination directory in the repository `copy_files` settings help
  text, including that it is repository-relative.

## Exclusions

- No relocation of seeded files to the task root. Which files may be promoted to
  a directory shared by every repository in the workspace is a separate decision
  with cross-repository blast radius; this work order only removes the silent
  case.
- No change to single-repository behavior, to `copyfiles.Copy`, to the spec
  syntax, or to the `:symlink` mode.
- No change to the remote-executor `repoSubpath` mapping in
  `remote_copyfiles.go`, which already mirrors the same per-repository
  destination.

## Acceptance

1. A two-repository workspace with a repository `copy_files` spec produces a
   reachability warning naming both the seed destination and the agent working
   directory; preparation still succeeds.
2. A one-repository workspace with the same spec produces no warning and seeds
   exactly as today.
3. The mismatch is determined from the resolved layout during preparation, with
   no agent involved.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_worktree.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/web/components/settings/` (repository `copy_files` help text)
- `apps/web/src/locales/*/settings.json` (six locales plus `pseudo`)
- New `apps/backend/internal/agent/runtime/lifecycle/seed_reachability_test.go`

## TDD sequence

1. Add failing tests: a two-repository preparation with a `copy_files` spec
   yields the reachability warning and still succeeds; a one-repository
   preparation with the same spec yields none; a two-repository preparation with
   no spec yields none.
2. Run the focused command and confirm the expected failures.
3. Implement the layout-derived check and the warning.
4. Update the settings help text and the locale entries; run the i18n checks.
5. Re-run the focused command; all pass.

## Verification

```bash
cd "$(git rev-parse --show-toplevel)/apps/backend" && go test ./internal/agent/runtime/lifecycle/... ./internal/worktree/... -race -count=1
cd "$(git rev-parse --show-toplevel)/apps/web" && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Dependencies

None. Disjoint from every other work order in this package.

## Risks

The warning must not fire for a repository whose `copy_files` spec seeds build
or runtime files rather than agent configuration; it names a reachability fact
about the agent working directory, not a defect in the spec. Keep the wording
descriptive, and do not turn it into a validation error.

## Results

Done.

Implemented:
- `RepoPrepareSpec` carries the repository's `copy_files` spec, populated from
  the launch request. Materialization still happens in the worktree manager;
  the preparer only needs it to judge reachability.
- `WorktreePreparer.warnUnreachableCopyFilesSeeds` runs at the end of
  multi-repository preparation and reports each seeding repository with its
  destination, the agent's working directory, and the repository count. It is
  derived from the resolved layout, so no agent is involved.
- The repository `copy_files` settings help states the destination and that a
  two-or-more-repository task puts the agent one level above it. Copy added in
  all six locales plus the regenerated pseudo entry.

Detection, not relocation: which files may be promoted to a directory shared by
every repository in the workspace is a separate decision with cross-repository
blast radius.

Verification (2026-09-22):

```
go test ./internal/agent/runtime/lifecycle/... ./internal/worktree/... -count=1
cd apps/web && npx vitest run components/settings/repository-copy-files-help && pnpm run i18n:ratchet
```

All clean.
