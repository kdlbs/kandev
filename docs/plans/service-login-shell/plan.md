---
created: 2026-10-26
status: in_progress
requirements:
  - REQ-CLI-SERVICE-LOGIN-SHELL-001
system_design:
  - ../../specs/cli/system-design/service-login-shell.md
legacy_specs: []
---

# Implementation Plan: Service login shell

## Overview

Repair [issue #4527](https://github.com/kdlbs/kandev/issues/4527) with explicit
login initialization for macOS user services. The issue is assigned to
`carlosflorencio`. The user selected the opt-in approach on 2026-10-26.
Implement the complete install/start/update contract first. Then establish
native macOS evidence and publish the operator instructions.

Task 01 implementation and public documentation are complete. Task 02 remains
in progress until native macOS and disposable-account acceptance is recorded.

## Evidence and root cause

Source revision: `b4e9cf5ede3545ca6a8e58165e580ad7c4988afa`.
The issue contains text logs and a prior maintainer investigation, with no images.

`serviceNodeToolBinDirs` captures only Node/npm/npx directories and
`npm_node_execpath`. `renderLaunchdPlist` adds the fixed launchd PATH.
`backendEnvForConfig` subsequently appends the account's `.local/bin`.
No stage imports a separate mise shims directory or shell exports.
When Node resolves through the shims directory, that directory is already captured.

A temporary `TestIssue4527SeparateToolDirectory` exercised the real renderer,
Node detection, backend environment builder, and a real `/bin/sh` subprocess.
It used temporary executable fixtures, without a running Kandev instance.
The separate-directory case could not execute the tool. The shared-directory
control executed it successfully. Both observations passed their assertions.
The test was removed after diagnosis.

Diagnostic command, run from `apps/backend`:

```bash
go test -tags fts5 ./internal/launcher -run '^(TestIssue4527SeparateToolDirectory|TestLocalAgentPathLaunchd|TestLocalAgentPathSelectedServiceAccount)$' -count=1 -v
```

Result: PASS, package time 0.058s. This was Linux evidence for generated
configuration and subprocess behavior, not native launchd or Zsh acceptance.
No persistent logs, service definitions, or account files were changed.

## Requirement reconciliation

The active `REQ-PLATFORM-LOCAL-AGENT-PATH-001` covers `.local/bin`, not arbitrary
tool directories or login initialization. Its current tests still pass.
The repair adds `REQ-CLI-SERVICE-LOGIN-SHELL-001`, with criteria 001.1 through 001.8.
CLI owns the explicit selection and its install/update lifecycle.
The [new design](../../specs/cli/system-design/service-login-shell.md) and
[ADR](../../decisions/2026-10-26-opt-in-service-login-shell.md) define the boundary.
The platform design links the exception without changing its shared helper.
No existing companion plan implements this new selection.

## Scope

### In scope

- macOS user services with explicit Bash or Zsh selection.
- Validation, reset, managed reinstallation, and update persistence.
- Structured plist arguments and native process replacement.
- Exported PATH/environment inheritance through existing consumers.
- Portable regressions, native macOS acceptance, and public instructions.

### Out of scope

- Default shell initialization, system services, Linux, Windows, and other shells.
- Interactive mode, profile editing, aliases/functions, project-specific activation.
- Additional fixed PATH directories, remote executor changes, and rendered UI changes.
- Package publishing, live user-service modification, and implementation during planning.

## Technical approach

`internal/launcher/service.go` gains the public shell argument and hidden `run`
action. A focused `service_shell.go` owns validation and saved selection.
Darwin-specific code owns process replacement. Existing shared PATH code remains intact.
The generated entrypoint remains Kandev, with structured arguments for the shell path.

`service_metadata.go`, `system/updates/install_state.go`, and
`service_self_update.go` all need the optional shell field. The intent writer
must retain it. Apply preflight must reject a plist/metadata shell mismatch.
Every package-manager plan must carry the selected shell through regeneration.
Omitted, explicit empty, JSON null, invalid, and explicit reset values have
different boundary-specific tests, as defined in the design.

### Compatibility matrix

| Surface | Identity and transport | Intended behavior | Evidence and fallback |
| --- | --- | --- | --- |
| macOS user, Bash/Zsh | native Kandev argv, absolute shell | opt-in login initialization | portable fixtures plus native tests |
| macOS user, no selection | existing direct argv | unchanged | renderer and legacy metadata controls |
| Homebrew/npm/npx Apply | user launchd, serialized intent | preserve selection | all three planner branches plus intent round trip |
| macOS system/Linux/Windows | existing manager paths | unchanged | reject shell argument before mutation |
| Unsupported or invalid shell | explicit selection | actionable error | validation tests, no silent fallback |
| Local probes/agents | inherited backend environment | receive exported values | real child discovery and execution |
| Remote executors | executor-owned environment | unchanged | no changes to remote startup code |

## Tests

Permanent regression coverage is implemented for the cases below. Darwin-only
cases still require execution on a disposable macOS login account.

| AC suffix | Test and file |
| --- | --- |
| 001.1, 001.2, 001.5, 001.7 | `TestServiceShellSelection` in `launcher/service_shell_test.go` |
| 001.2, 001.6 | `TestServiceShellPlistArguments` in the same file |
| 001.3, 001.4, 001.6, 001.7 | `TestServiceShellSubprocess` in the same file |
| 001.3 | `TestLocalAgentPathLoginShellSeparateTools` in `launcher/local_agent_path_test.go` |
| 001.5, 001.7 | `TestServiceShellSelfUpdateRoundTrip` in `launcher/service_native_update_test.go` |
| 001.5, 001.7 | `TestWriteApplyIntentPreservesNativeShell` in `system/updates/apply_test.go` |
| 001.5, 001.7 | `TestServiceShellMetadataMismatch` in `system/updates/install_state_test.go` |
| 001.3, 001.4, 001.6, 001.7 | `TestServiceShellDarwinLoginEnvironment` and `TestServiceShellDarwinLaunchdLifecycle` in `launcher/service_shell_darwin_test.go` |
| 001.8 | Public command examples and recovery acceptance in Task 02 |

The first red test confirmed that `parseServiceArgs` rejected
`install --shell /bin/zsh`. The implementation then reproduced and covered the
separate-directory behavior through real `exec.LookPath` and subprocess fixtures,
with a shared-directory control and the existing `.local/bin` regressions.

## End-to-end evidence

This is a CLI/service contract. No Playwright scenario or web project applies.
Task 01 connects generated environment, discovery, and executable subprocesses.
Darwin-only tests exercise real shell initialization and launchd process lifetime,
but they have not run on macOS. A disposable-account Kandev startup, stop/restart,
and configured-entrypoint observation also remains pending. Native evidence is
mandatory before completion.

## Work orders

- [x] [Task 01: Implement persistent service shell selection](task-01-service-shell.md)
- [ ] [Task 02: Verify native macOS behavior and document operation](task-02-native-acceptance.md)

Execute sequentially. Task 02 depends on Task 01. Native macOS acceptance is the
remaining gate and requires a disposable logged-in account.

## Verification results

Diagnostic reproduction: PASS as recorded above. Task 01 implementation checks:
PASS on Linux. `make -C apps/backend build`: PASS. Changed-Go lint: PASS.
`go test -trimpath -tags fts5 ./internal/launcher ./internal/system/updates -count=1`:
PASS. Spec catalog, specification lint, public docs validation, and
`git diff --check`: PASS.

The Darwin launcher test cross-compile was attempted with CGO disabled but did
not reach the launcher package: existing SQLite files require CGO-provided
`sqlite3.Error` and `sqlite3.ErrConstraintUnique` identifiers. Native macOS test
execution remains pending on a suitable host.

Design-package checks on 2026-10-26:

- `python3 scripts/list-docs.py validate`: PASS, 404 decisions and 1,742 specifications.
- `python3 scripts/lint-spec-files.test.py`: PASS, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: PASS.
- `.github/scripts/pr-docs.cjs:validateCoverage`: PASS for both work orders,
  using their workspace content and the planned `service.go` runtime change.
  This was a local prospective coverage check, not a GitHub PR result.
- `git diff --check` and the package-specific diff check: PASS.
- `git status --short -- docs/plans/service-login-shell`: new package present.

Task 01 is committed on the feature branch and under review in PR #4821. Public
documentation is included in Task 02; native OS and installed-runtime evidence
is still required.

### PR fixup, 2026-10-27

Rebased onto `main` at `af92a5f6d4e` (the base advanced once more between the
fetch and the rebase) with no conflicts. The first check run for the previous
head failed `E2E Shard 26/28` on
`Task List › sort and group options are deep-linkable and persisted`
(`apps/web/e2e/tests/task/task-list.spec.ts`): the backend reported
`tasks_list_group` as `none` after the selection.

Cause: the tasks list sent its sort/group preference snapshots as independent
fire-and-forget PATCHes, so an older snapshot could land after a newer one, and
the spec also read the backend once, immediately after the interaction. The
defect is outside the CLI feature, but it blocked a required current-head check,
so it is fixed here:

- `apps/web/app/tasks/tasks-page-client.tsx` serializes preference writes, so
  the newest selection always reaches the server last.
- `apps/web/e2e/tests/task/task-list.spec.ts` polls the persisted preference
  instead of a single read, matching the `task-list-facet.spec.ts` precedent.
- `apps/web/app/tasks/tasks-page-client.facet-selection.test.tsx` covers the
  ordered writes; it fails with two concurrent PATCHes before the change.

Verification on the rebased branch:

- `(cd apps/web && pnpm exec vitest run app/tasks)`: PASS, 70 tests.
- `(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/task/task-list.spec.ts)`: PASS, 9 tests.
- `(cd apps/web && NODE_OPTIONS=--max-old-space-size=10240 pnpm run typecheck)`: PASS.
- `(cd apps/web && pnpm exec eslint app/tasks/tasks-page-client.tsx app/tasks/tasks-page-client.facet-selection.test.tsx e2e/tests/task/task-list.spec.ts)`: PASS.

The base advanced again after that push. No further rebase was required: the
changed-path sets of the base advance and this branch are disjoint, and
`git merge-tree --write-tree <current-base> <head>` builds a conflict-free
synthetic tree whose versions of this branch's changed files are byte-identical
to the head.

## Risks

- Startup files can hang, exit, print sensitive output, or change unrelated exports.
- Bash and Zsh startup files differ. Login mode does not promise terminal parity.
- macOS background-item labels depend on native OS behavior beyond plist argv.
- All three metadata representations must retain the value during updates.
- Native acceptance requires a disposable macOS login account and launchd session.
- The selected scope does not resolve tools configured only in interactive files.
