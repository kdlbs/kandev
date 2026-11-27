---
id: "01-preserve-local-branch"
title: "Resume a retained local task branch after managed refresh"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.5
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 01: Preserve local task branch recreation

## Summary and scope

Allow the retained task branch without an origin tracking ref after a successful
managed refresh. Change `internal/worktree/manager_lifecycle.go` and extend the
existing archive/unarchive regression in `manager_recreate_recovery_test.go`.
Add explicit-selection negative coverage in `manager_recreate_refresh_test.go`.
Clarify the existing agent-resume system design.

Do not relax initial checkout, PR snapshot, remote contribution, missing-local-ref,
or branch-replacement behavior. No browser or rendered UI changes are needed.
There are no implementation dependencies or new architectural decisions.

## Acceptance conditions

1. Archive/unarchive recreation succeeds after a real fetch with the original
   local branch, path, commit, reference visibility, and active record preserved.
2. An explicit checkout branch or PR snapshot with no refreshed source still
   fails without materializing a checkout or changing the local branch head.
3. Existing remote-ref selection, recovery-head restoration, and missing-branch
   tests continue to pass.

## Verification commands

From `apps/backend`:

```sh
GOWORK=off go test ./internal/worktree -run '^(TestCreate_RestoresReleasedWorktreeAfterArchive|TestRecreate_ManagedRefreshRequiresSelectedRemoteRef)$' -count=1 -v
GOWORK=off go test -race ./internal/worktree -count=1
```

From the repository root:

```sh
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Normal commit hooks additionally run Go lint for changed packages.

## Results

The archive/unarchive regression failed before the production change with
`required fetched remote ref "origin/feature/archived-work-..." is missing`.
After the correction, the archive/unarchive regression and both explicit-selection
cases passed. The full worktree package passed with the race detector in 62.916s.
Documentation catalog validation, full specification lint, and `git diff --check`
passed. Public docs need no change: the existing recovery flow and controls are
unchanged, and the correction restores their documented branch recovery behavior.
