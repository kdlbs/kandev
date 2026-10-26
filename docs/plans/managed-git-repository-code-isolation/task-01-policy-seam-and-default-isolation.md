---
id: "01-policy-seam-and-default-isolation"
title: "Policy seam and default isolation"
status: draft
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-GITISOLATION-001
acceptance_criteria:
  - AC-AGENTS-GITISOLATION-001.1
  - AC-AGENTS-GITISOLATION-001.2
  - AC-AGENTS-GITISOLATION-001.3
  - AC-AGENTS-GITISOLATION-001.4
  - AC-AGENTS-GITISOLATION-001.5
  - AC-AGENTS-GITISOLATION-001.6
  - AC-AGENTS-GITISOLATION-001.7
  - AC-AGENTS-GITISOLATION-001.8
  - AC-AGENTS-GITISOLATION-001.9
  - AC-AGENTS-GITISOLATION-001.10
  - AC-AGENTS-GITISOLATION-001.11
  - AC-AGENTS-GITISOLATION-001.13
  - AC-AGENTS-GITISOLATION-001.14
  - AC-AGENTS-GITISOLATION-001.15
system_design:
  - ../../specs/agents/system-design/managed-git-repository-code-isolation.md
---

# Task 01: Policy seam and default isolation

## Summary

Apply the managed Git policy in `startManagedGit` (the point where the final
`Dir`, `Env` and context are known) and through `subproc.ManagedGitArgs` for the
`gitinit` helper, with `subproc.WithGitHooks(ctx, enabled)` as the only way to keep
hooks. Default-off hooks, fsmonitor, textconv and external diff, `ext` transport,
non-allowlisted filters and merge drivers.

## In scope

- Policy pairs placed after caller global options, caller `-c`, `-ckey=value` and
  `--config-env` stripping for policy-owned keys, sorted vector, refusal of
  `blame` and of `--textconv`, `--filters`, `--ext-diff` arguments.
- Allowlist by canonical command form for `lfs` (including `--skip` and legacy
  forms), `git-crypt`, and `git-crypt-<key>`; required non-allowlisted filters
  refuse content-writing commands and are neutralized for filter-reading commands;
  names containing `=` fail closed; the three subcommand lists and the build-time
  classification test over an explicit wrapper inventory; dry-run pushes get the null hooks path; the Kandev-owned `pre-push` hooks directory for managed
  `push` when hooks are off and an allowlisted `lfs` filter is configured.
- The `gitinit` helper path: the process starter passes a non-Git command through
  unmodified and the helper applies `ManagedGitArgs(ctx, env, gitArgs)` after its
  directory change, with errors on standard error and a non-zero exit.
- Enumeration limits (5 s, 1 MiB, 256 characters) and a receipt of measured added
  latency per status poll.
- Driver enumeration with the default-deny subcommand list and fail-closed errors.
- Removal of the now-redundant pins in `submodule.go`, `preservation.go`,
  `local_repository_initialization.go`; submodules never carry the hooks mark.
- Go tests planting a marker-writing hook, an fsmonitor command, a textconv
  driver, an external diff, a merge driver and an unknown filter in a temp
  repository and worktree, covering commit, push to a local bare remote, pull
  (merge and rebase), merge, reset, revert, branch switch, status poll, diff and
  `worktree add`; an LFS-style allowlisted filter still applying; the hooks mark
  running hooks; enumeration failure refusing the command; the agent environment
  carrying no policy value.

## Out of scope

The repository setting, API, propagation to agentctl, UI and public docs.

## Acceptance

`AC-AGENTS-GITISOLATION-001.1` through `001.11`, `001.13`, `001.14` and `001.15`.

## Verification

```bash
cd apps/backend
go test -trimpath ./internal/common/subproc -count=1
go test -trimpath ./internal/worktree ./internal/agentctl/server/process -count=1
bash ../../scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

## Likely files and risks

`internal/common/subproc/git_lifecycle.go` (`startManagedGit`), `git_environment.go`, new
`git_policy.go`, `internal/worktree/{submodule,preservation}.go`,
`internal/task/service/local_repository_initialization.go`,
`internal/task/gitinit/command_descriptor.go`. Risk: argument-vector assertions in
existing tests; one extra process per filter-capable command on the status poll.

## Results

Pending.
