---
created: 2026-10-22
status: implemented
requirements:
  - REQ-EXECUTORS-PLUGIN-002
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
legacy_specs: []
---

# Implementation Plan: Plugin materialization managed credentials

## Overview

Repair [issue 4677](https://github.com/kdlbs/kandev/issues/4677) using the existing
executor-owned [requirements](../../specs/executors/requirements/remote-executor-plugins.md)
and [design](../../specs/executors/system-design/remote-executor-plugins.md#repository-materialization-credentials).
The user requested AFK delivery through merge. Repository instructions require
this separate design handoff before implementation; all work remains in the primary session.

## Evidence and root cause

Read-only source tracing confirms that `handleWorkspaceMaterializeRepository`
calls the materialization pipeline without the instance environment. Both
`materializeGitOutput` and `materializeCheckoutGit` construct commands with a nil
`Cmd.Env`, inheriting agentctl's process environment. The plugin provider only
receives bootstrap metadata; the broker lease instead reaches `CreateRequest.Env`
and `InstanceConfig.AgentEnv`. Process-managed workspace Git already uses instance
environment snapshots. The issue contributor independently reports a successful
private clone on a real AWS Lambda MicroVM after correcting this gap, and warns
that replacing the environment wholesale can drop the installed helper path.
That external result is supporting evidence, not a live verification of this branch.

Smallest reproduction: start an instance whose environment contains a valid managed
broker contract, leave that contract absent from the agentctl process environment,
and POST a credential-free private repository locator to
`/workspace/materialize-repository`. Current code returns 422 and never redeems
the instance lease. The implemented regression reproduced that failure before the correction.

## Scope

### In scope

- Current instance credentials for both materialization Git runners, including fetch and sparse checkout.
- Preserve installed helper tooling and unrelated Git config; reject stale cross-session authorization.
- Deterministic authenticated real-Git HTTP integration evidence and operator documentation.

### Out of scope

- Provider protocol or SDK changes, production cloud plugin implementation, credential-default changes.
- New credential grants, broker policy changes, new flags, database migrations, or rendered UI changes.

## Technical approach

Use a detached current-instance environment snapshot and an explicit per-operation
Git runner, following the paired design. Avoid mutable global test hooks and
process environment writes. Preserve helper path captured at manager construction;
use `gitconfigenv` for indexed config and the existing subproc seam for execution.
Instance admission strips inherited managed authorization before merging the current
request while preserving installed tools and unrelated config. An existing ADR
already assigns core ownership of materialization; this repair
restores that boundary and needs no new architecture decision.

Compatibility coverage:

| Runtime and identity | Expected behavior | Evidence |
| --- | --- | --- |
| Plugin-started agentctl, managed instance lease | Private HTTPS clone/fetch uses instance lease and installed helper | New authenticated Git/API regression |
| Docker, Kubernetes, SSH with startup helper tooling | Current instance authorization wins; helper remains available | Environment precedence regression |
| No managed lease, public repository | Existing public clone/reuse works | Existing materialization suite |
| Executor credential mode | Existing executor credentials; no automatic managed grant | No-lease and stale-authorization cases |
| Missing/denied authorization | Sanitized failure, cleanup, no credential persistence | Denied broker and response assertions |

## Tests

Add `workspace_materialize_credentials_test.go` in agentctl's API package:
`TestWorkspaceMaterializeRepository_ManagedCredentials` must fail before the fix
because Git cannot redeem the instance lease. Exercise initial clone and reuse
fetch against a private local HTTP Git fixture requiring helper credentials.
Cover missing process broker env, process-only installed helper path, two distinct
instance snapshots, rejected credentials, and no-lease behavior. Inspect the final
Git config for absence of credentials. Exercise the sparse-checkout stdin runner
with an instance-only Git config marker. Map these to AC-EXECUTORS-PLUGIN-002.5,
AC-EXECUTORS-PLUGIN-002.6, and AC-EXECUTORS-PLUGIN-002.7.

Add focused process-manager tests if an exported snapshot/helper preparation
method is introduced: detached copies, configuration refresh, indexed config
preservation, malformed block handling, and installed-helper preservation.

## E2E tests

The new Go HTTP/API regression must execute real Git, a credential-helper child,
and a deterministic credential broker/private Git fixture. A mocked Git command
or a mere environment assertion is insufficient. This is the direct end-to-end
boundary for the defect; no browser interaction changes. Existing packaged-plugin
Playwright coverage runs as an additional provider compatibility smoke check;
it does not prove private cloning or real AWS compatibility.

## Work orders

- [x] [Task 01: Repair and verify materialization credentials](task-01-materialization-credentials.md)

## Verification results

Design validation completed on 2026-10-22:

- `python3 scripts/list-docs.py validate`: passed (396 decisions, 1699 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed after keeping the existing design below its size limit.
- Repository `.github/scripts/pr-docs.cjs` `validateCoverage` API: passed (`covered`, no errors), using the planned materialization runtime path to exercise reference validation before code changes.
- `git diff --check -- docs/plans/plugin-materialize-managed-credentials docs/specs/executors/requirements/remote-executor-plugins.md docs/specs/executors/system-design/remote-executor-plugins.md`: passed.
- `git status --short -- docs/plans/plugin-materialize-managed-credentials`: confirms the untracked package is present and uncommitted.

The shell does not expose Node on PATH; the coverage preflight used
`/home/jcfs/.local/share/mise/installs/node/24/bin/node`. Activate the installed
mise toolchain before running the work order's Node/pnpm commands.
The user subsequently requested implementation. Production changes and all
work-order checks are complete; exact results are recorded in
[Task 01](task-01-materialization-credentials.md#results). Public documentation
now explains private-repository credential prerequisites. PR review and merge
continue under the original end-to-end delivery authorization.

## Risks

- Naive indexed config overlays can truncate or retain stale credential helpers.
- Startup lease fallback can silently cross session authorization boundaries.
- Replacing the environment can remove PATH, HOME, or the installed helper path.
- API Git integration is reproducible without external credentials; production
  cloud provider validation requires that provider and is not claimed by this package.
