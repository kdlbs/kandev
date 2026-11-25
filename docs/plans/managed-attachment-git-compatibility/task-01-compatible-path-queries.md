---
id: "01-compatible-path-queries"
title: "Use compatible attachment path queries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-008
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.1
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.6
system_design:
  - ../../specs/tasks/system-design/current-managed-attachment-recovery.md
---

# Task 01: Use compatible attachment path queries

## Summary

Eliminate false ownership rejection caused by `rev-parse --path-format=absolute`
on Git 2.30. Preserve exact managed-clone validation and prove the registered
MCP GitHub URL consumer against real Git and disposable repository content.

## In scope

- Mark this work order `in_progress` before permanent test or production edits.
- Write `TestManagedAttachmentGitPathCompatibility` and
  `TestManagedBranchGitHubURLGitCompatibility` first. Observe the old-Git URL
  regression fail with the exact ownership mismatch before changing production.
- Use current Git to create real fixture repositories. For application calls,
  wrap Git so `--path-format=absolute` behaves as an unknown option on Git 2.30,
  preserving zero exit status and echoed flag plus path. Delegate other commands.
  New compatible queries must pass through unchanged. Reuse actual Git path
  queries; never hardcode the expected checkout path in command output.
- Exercise the canonical `github_url` input through the registered guarded
  MCP handler and service repository resolution, with fixture credentials and
  command-local transport rewriting to disposable local content. Include a
  modern URL and explicit-local control. Read the returned checkout and verify
  canonical associations/inventory and exact task/session credential scope.
- Remove `--path-format` from this attachment query seam. Preserve absolute
  top-level/Git-dir results; resolve a relative common-dir result against the
  admitted checkout and compare physical paths after existing control checks.
- Preserve exactly one terminal output newline removal, lexical persistence,
  origin matching, clone guards and all current refusal/compensation behavior.

## Out of scope

Other Git inspectors, new ownership rules or diagnostics architecture, provider
auth changes, global Git-version policy, UI, migrations, runtime flags, remote
executors, publication and deployment. No external incident is declared resolved
solely because this independent regression passes.

## Acceptance

1. A valid managed GitHub URL reaches successful registered MCP attachment when
   inspection follows Git 2.30 behavior. The current-Git URL and explicit-local
   controls still succeed; responses expose readable checkout content, exact
   paths and inventory without changing the agent CWD.
2. Compatible path queries preserve space/newline names and trusted parent aliases,
   while foreign origins and redirected Git/control paths still fail without
   foreign-content changes. No broad trim or path-output splitting is introduced.
3. The exact checks below pass. Record RED/GREEN evidence and platform/test-double
   limits here and in the plan; mark this order `done` only after completion.

## Verification

Run sequentially from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/repoclone -run 'TestManagedAttachment' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/backendapp -run 'TestManagedBranchGitHubURLGitCompatibility|TestManagedBranchAttachmentConsumers|TestManagedSourceAttachmentConsumers|TestManagedAttachmentCompatibilityConsumers' -count=1)
bash scripts/lint-go-changed apps/backend/internal/repoclone/managed_attachment_path.go apps/backend/internal/repoclone/managed_attachment_git_compatibility_test.go apps/backend/internal/backendapp/managed_branch_git_compatibility_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The new focused regression must be run before and after the correction. Existing
security cases must continue to run, not be weakened to make the new case pass.
Use native-platform evidence when available and report unsupported wrapper or
filesystem capabilities honestly. No actual old Git is required for deterministic
permanent tests; the plan already records actual Git 2.30 diagnostic evidence.

## Files likely touched

- `apps/backend/internal/repoclone/managed_attachment_path.go`
- `apps/backend/internal/repoclone/managed_attachment_git_compatibility_test.go` (new)
- `apps/backend/internal/backendapp/managed_branch_git_compatibility_test.go` (new)
- This work order and `plan.md` for status/results.

## Dependencies

None. Existing active requirements, design and accepted ownership decision apply.

## Risks

Relative paths must be resolved against the inspected checkout, never the process
CWD. A physical comparison must not authorize redirected content. Fixture transport
and credentials do not prove external GitHub auth or all production lifecycle
behavior; preserve those limits in results. Avoid shell-only helpers where an
existing portable test-helper pattern is available.

## Parallelism

`sequential`

## Inputs

- [Requirement and ACs](../../specs/tasks/requirements/managed-source-attachment.md)
- [Design, Git path query compatibility](../../specs/tasks/system-design/current-managed-attachment-recovery.md#git-path-query-compatibility)
- [Plan and actual Git diagnostic receipts](plan.md#confirmed-defect-and-evidence)
- `apps/backend/internal/repoclone/managed_attachment_path_test.go`
- `apps/backend/internal/repoclone/managed_attachment_compatibility_test.go`
- `apps/backend/internal/backendapp/managed_source_attachment_test.go`
- `apps/backend/internal/backendapp/managed_branch_attachment_test.go`
- Scoped backend guidance and `/tdd` backend testing conventions.

## Results

Implemented the compatible path queries and both permanent regression suites.

- RED: `TestManagedAttachmentGitPathCompatibility` rejected all four valid
  old-Git fixtures with the ownership sentinel before the fix.
- RED: `TestManagedBranchGitHubURLGitCompatibility` failed only `legacy-url`
  with the exact reported `INTERNAL_ERROR`; current URL and local controls passed.
- GREEN: both new focused suites passed after the production correction.
- The required `repoclone` race command passed (1.536s).
- The required `backendapp` race command passed (56.494s).

The exact required checks passed:

```text
(cd apps/backend && go test -trimpath -race ./internal/repoclone -run 'TestManagedAttachment' -count=1)
PASS (1.536s)
(cd apps/backend && go test -trimpath -race ./internal/backendapp -run 'TestManagedBranchGitHubURLGitCompatibility|TestManagedBranchAttachmentConsumers|TestManagedSourceAttachmentConsumers|TestManagedAttachmentCompatibilityConsumers' -count=1)
PASS (56.494s)
bash scripts/lint-go-changed apps/backend/internal/repoclone/managed_attachment_path.go apps/backend/internal/repoclone/managed_attachment_git_compatibility_test.go apps/backend/internal/backendapp/managed_branch_git_compatibility_test.go
PASS (0 issues)
python3 scripts/list-docs.py validate
PASS
python3 scripts/lint-spec-files.py --all
PASS
git diff --check
PASS
```

The work-order requirement/AC and plan/design reference checks also passed.
Public docs retain the documented attachment semantics; internal design and
delivery records are updated. No new Git minimum-version policy is published.
The repository's Node PR-documentation coverage preflight passed during delivery
after locating the Mise runtime. The user subsequently authorized delivery
through merge; remote CI/review and merge receipts are recorded in the PR and
session handoff. No delegation or live-instance mutation is part of delivery.

These are native Linux tests with a POSIX old-Git protocol wrapper, real Git
repositories and the existing persisting test worktree manager. They do not
establish native Windows/macOS or external GitHub authentication acceptance.
