---
created: 2026-10-01
status: complete
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001
system_design:
  - ../../specs/tasks/system-design/remote-contribution-tasks.md
legacy_specs: []
---

# Implementation Plan: Preserve Worktree Credential Paths

## Overview

Preserve the Git repository path when worktree preparation calls the managed
credential helper. One work order restores the existing contribution checkout
contract without changing credential authority.

The task system owns this repair because the failed outcome is contribution
task preparation. Existing criterion `AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.2`
already requires preparation at the validated head. Criterion 001.3 preserves
ordinary repository behavior. No new product requirement or architecture
decision is necessary. The existing design's Permissions section and
[contribution binding decision](../../decisions/2026-08-04-remote-contribution-bindings.md)
remain authoritative.

## Evidence and root cause

Issue [#4023](https://github.com/kdlbs/kandev/issues/4023) initially attributed
the failure to missing inherited metadata. The reporter's
[clarification](https://github.com/kdlbs/kandev/issues/4023#issuecomment-5901903062)
instead identifies an explicit PR URL, `new_workspace`, a same-repository PR,
and a complete contribution binding.

The confirmed failure path is:

1. `Executor.configureGitCredentialEnvironment` appends
   `credential.useHttpPath=true` to the indexed Git configuration.
2. `buildWorktreeCreateRequest` passes the environment through
   `checkoutCredentialEnvironment` in `repository_checkout_options.go`.
3. That filter retains host-specific `credential.https://*.useHttpPath`
   entries, but drops the global key that the executor actually emits.
4. Git omits the repository path from its HTTPS credential request.
5. `newGitHubCredentialBrokerClientForInput` cannot match that request against
   a repository scope. It returns the reported error before contacting the broker.

The existing checkout environment test uses a host-specific key. Its fixture
does not represent the executor's emitted configuration. The same filter is
present in the `v0.96.0` source.

### Reproduction results

Two temporary tests used real Git, a freshly built agentctl helper, and a local
HTTP broker with synthetic credentials. No external credentials or live
instance data were used.

- `TestRepro4023RealGitCredentialPathFiltering` exercised the real lifecycle
  filter. Across four repetitions, the full environment succeeded and the
  filtered environment produced the exact scope error with zero broker calls.
  Restoring only the omitted key made the filtered environment succeed.
- `TestRepro4023ContributionFetchWithFilteredCredentials` exercised
  `worktree.Manager.Create` against a local authenticated HTTPS Git fixture.
  Three contribution checkouts warmed the clone with the key present. A fourth
  checkout with the filtered environment produced the exact reported error:

```text
fetch contribution branch: git repository does not match any credential lease scope
error: cannot run exit 1: No such file or directory
fatal: could not read Username for 'https://github.com': terminal prompts disabled
```

Restoring the key allowed the same fourth task/session to check out the exact
bound SHA. The binding and lease did not change. The worktree fixture used the
filter output shape confirmed by the lifecycle test.

The experiment deliberately changed the configuration between warmup and
failure. It proves the defect and correction, not a spontaneous timing race.
The reporter's delayed onset and automatic recovery remain unverified.
Existing Git configuration or another preparation path can mask the defect,
but this investigation does not establish which occurred on that installation.

Temporary test sources were copied outside the repository for session-local
reference, then removed from the source tree. Permanent regressions belong to
the implementation work order.

## Scope

### In scope

- Preserve the executor-emitted path setting through the checkout filter.
- Cover both global and existing host-specific true values.
- Preserve exclusions for profile controls, secrets, and unrelated Git settings.
- Verify repository-scoped helper behavior with real Git and a local fixture.

### Out of scope

- Parent metadata inheritance, PR associations, branch policy, or migrations.
- Broader credentials, relaxed scope matching, retries, or lease timing changes.
- A claim that the reporter's automatic recovery is reproduced.
- UI, translations, public configuration, or new operator instructions.

## Technical approach

Change only the allowlist in
`apps/backend/internal/agent/runtime/lifecycle/repository_checkout_options.go`.
Retain `credential.useHttpPath` when its value is `true`, alongside the existing
host-specific form. Preserve the managed helper entries and indexed ordering.
Do not copy the whole profile environment or change scope validation.

| Path | Intended behavior | Evidence |
| --- | --- | --- |
| Managed HTTPS, same-repository GitHub PR | Retain path and use canonical scope | Real Git/helper integration |
| Managed HTTPS, fork PR | Retain path and select validated source scope | Helper scope-selection tests |
| Existing host-specific path setting | Preserve current behavior | Filter unit tests |
| Ordinary managed repository | Preserve path and scope checks | Helper integration control |
| Unrelated profile settings or secrets | Exclude from host checkout | Filter exclusion tests |
| Executor-owned credentials | Preserve current behavior | Existing environment tests |

The shared filter also carries managed provider credentials. This repair does
not claim new provider support or change any provider's authorization policy.

## Tests

Criteria 001.2 and 001.3 map to the tests in
[Task 01](task-01-preserve-credential-path.md). The regression must consume
the global key emitted by the executor. A host-specific-only fixture cannot
prove the correction.

## End-to-end evidence

The backend integration crosses environment filtering, real Git, the real
credential helper, a local broker, and authenticated fetch. This proves the
preparation boundary without an unrelated browser test. It does not exercise
the entire MCP task-creation or agent-start workflow.

## Work orders

- [x] [Task 01: Preserve repository paths in checkout credentials](task-01-preserve-credential-path.md)

## Verification results

Diagnostic commands passed before production changes:

```bash
(cd apps/backend && go build -o /tmp/kandev-4023-agentctl ./cmd/agentctl)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestRepro4023' -count=1 -v)
(cd apps/backend && go test -tags fts5 ./internal/worktree -run '^TestRepro4023' -count=1 -v)
```

The first worktree diagnostic attempt failed fixture validation because its
source path did not match its URL. Correcting that fixture produced the result
above. The temporary diagnostic tests and binary have since been removed after
equivalent permanent regressions were added below.

Package checks passed on 2026-10-01:

- `python3 scripts/list-docs.py validate`: 327 decisions and 1,243 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/plans/worktree-credential-path`: passed.
- `git status --short`: only this untracked plan directory remained.
- `.github/scripts/pr-docs.cjs:validateCoverage`: covered, with no errors.
  The preflight used both package files, their referenced requirement/design,
  and the proposed production path as a simulated changed-file input.

## Implementation verification

The regression tests first failed as expected: the filter returned three
indexed entries and omitted `credential.useHttpPath`, then real Git reported
`git repository does not match any credential lease scope` before the broker
was contacted. Preserving the global true value made both tests pass.

The completed work-order commands passed on 2026-10-01:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(BuildWorktreeCreateRequest|CheckoutCredentialEnvironment|SetupScriptEnvironment)' -count=1)
(cd apps/backend && go test -tags fts5 ./cmd/agentctl -run '^TestGitHubCredentialHelper' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/worktree -run '^TestCreateWorktree_RemoteContribution' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor -run '^TestConfigureGitHubCredentialBroker$' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
make -C apps/backend build
```

The worktree regression passed three authenticated contribution checkouts
through a local HTTPS proxy and synthetic broker, each at the provider's exact
head SHA. The test-built agentctl helper used a repository-scoped lease; no
external Git transport or credentials were used.

The temporary reproduction sources, helper binary, and reporter-evidence
draft under `/tmp` were removed after this permanent coverage was added.

Issue #4023 is assigned to `carlosflorencio`. The package is unstaged and
uncommitted. The production change and permanent regressions are recorded in
the completed work order above.

## Risks

- Broadening the allowlist beyond the true path setting can expose profile controls.
- Host or global Git configuration can hide the regression in an unisolated test.
- Scope/path mismatches for other reasons must remain errors.
- The reporter's installation can have an additional cause beyond this confirmed defect.
