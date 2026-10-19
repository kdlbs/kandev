---
created: 2026-10-14
status: implemented
requirements:
  - REQ-EXECUTORS-SSH-PASSTHROUGH-001
  - REQ-EXECUTORS-SURVIVAL-005
system_design:
  - ../../specs/executors/system-design/ssh-passthrough.md
legacy_specs: []
---

# Implementation plan: SSH passthrough

## Overview

Run SSH passthrough in the remote controller's PTY and carry terminal I/O
through the authenticated SSH forward. Deliver the controller API first,
then remote launch and I/O, then recovery, then real SSH browser evidence.
The user selected remote support and instructed us to assume current versions.

Implementation is complete. Work orders record the changes and verification.

## Scope

### In scope

- Remote CLI startup without a prepare script or host CLI installation.
- MCP preparation on the target, remote terminal streams, prompt and event identity.
- Reconnect, Stop, reset/model relaunch, explicit Resume, and uncertain-result handling.
- Existing desktop and phone terminal flows, real SSH tests, and public executor docs.

### Out of scope

New passthrough support for other remote executors, reverse MCP tunnels, CLI
installation, ACP journal adoption, and terminal visual redesign.
No Proxmox-specific or Tailscale-specific implementation is justified by current evidence.

## Root cause and reproduction

At base `73c5ddb8c`, SSH publishes a remote controller port but no standalone
port. Real Claude command preparation returns
`standalone port unavailable for passthrough MCP config`. Its bootstrap
projection returns `unknown` and `The bootstrap operation could not be completed.`

The PTY path independently uses the standalone runner. Two temporary tests
proved a missing host directory fails and a matching directory executes a
command on the host without an SSH connection. Both reproduction tests were
removed after their test-owned resources were cleaned up.

The existing `TestResolveLaunchAuthToken` suite passes. Fix `25eba37df` (#3007)
already addresses the historical non-Docker token-selection regression.
The reporter's exact missing-token cause remains unconfirmed.

## Technical approach

The [system design](../../specs/executors/system-design/ssh-passthrough.md) owns
the API, identity, materialization, and recovery contracts. The
[ADR](../../decisions/2026-10-14-ssh-passthrough-process-ownership.md) records why
agentctl owns the remote PTY.

1. Add an instance-owned wrapper around `InteractiveRunner`, typed controller
   DTOs, authenticated passthrough routes, and matching `agentctl.Client` methods.
2. Add selected-transport routing in lifecycle. Extract shared config I/O into
   `internal/common/passthroughconfig`. Use remote paths and the remote MCP URL.
   Route the existing gateway agent terminal through the remote binary stream.
3. Complete all lifecycle consumers and persistence allowlists. Resume inspects
   the actual remote passthrough process before the ACP status path can decide
   that no agent exists. Reject unknown liveness or missing credentials.
4. Exercise disposable SSH targets and phone contexts. Add bounded mock TUI
   evidence and document supported operator behavior only after implementation.

No SQL schema, package dependency, or default profile changes are planned.

### Compatibility matrix

| Executor shape | Intended behavior | Evidence |
| --- | --- | --- |
| Local/worktree | Existing local runner and standalone survival exclusion | Local passthrough and terminal regressions |
| SSH, capable helper | Remote native CLI, structured env, remote MCP resources | Controller tests and real SSH browser tests |
| SSH, old helper | Capability error with no host execution | HTTP 404/version rejection tests |
| SSH, uncertain controller/token | Block input and replacement, retain retry evidence | Resume/auth/liveness regressions |
| Other remote executor | No new support, no implicit SSH adapter or local fallback | Transport-selection negative tests |

Existing native passthrough agents are supported only when their CLI exists on
the target and their declared strategy/options are supported. Test Claude,
Codex, OpenCode, and project-file emission with fixtures. Mock TUI E2E proves
the shared transport. It does not claim live provider/version validation.

## Terminal interaction preview

Desktop keeps its existing composition. Phone Chat exposes the existing shell
keybar for agent passthrough. The keybar owns a session-bound agent sender and
reserves its existing 44-pixel height above task navigation.

`UI-01: Desktop agent terminal`, entry `/t/<task-id>`, SSH passthrough active:

```text
Task workspace
+---------------------------------------+
| Agent terminal                        |
| Remote CLI output                     |
| > user input                          |
+---------------------------------------+
```

`UI-02: Phone Chat terminal`, same task, narrow touch viewport:

```text
+-----------------------------+
| Task / Session              |
| Chat                       |
| Remote CLI output           |
| > user input                |
| [Ctrl] [Esc] [Tab] [...]     |
| Existing task navigation    |
+-----------------------------+
```

Terminal content owns scrolling. The existing phone keybar follows the visual
viewport and safe area. Chat remains the direct entry point. No new overlay or
desktop pane is introduced. Preserve manual input routing, canvas rendering,
accessible keybar names, and touch targets. ASCII spacing and example text are
illustrative. Existing labels remain localized.

AC-EXECUTORS-SSH-PASSTHROUGH-001.3, .4, and .13 require the same remote outcome
across these surfaces. Verify accepted prompt text, interrupt, resize, reconnect,
and no document horizontal overflow. Keyboard input receives no new animation.

## Tests

All names below are proposed permanent tests. Implement them through TDD.

| Criteria | Regression evidence |
| --- | --- |
| .1, .2, .5 | `TestSSHPassthroughBootstrapUsesRemoteMCP`, `TestSSHPassthroughNeverStartsHostCLI`, `TestSSHPassthroughEnvironment` |
| .3, .4, .13 | `TestRemotePassthroughTerminalBridge`, `TestRemotePassthroughDisconnectPreservesProcess` |
| .6, .8 | `TestRemotePassthroughInitialPromptBoundary`, `TestRemotePassthroughStaleEvents`, `TestRemotePassthroughInputDedup` |
| .7 | `TestRemotePassthroughConcurrentStart`, `TestRemotePassthroughSiblingIsolation` |
| .9 | `TestRemotePassthroughStopReapsDescendants`, `TestRemotePassthroughResetAndModelChange` |
| .10, .11 | `TestSSHPassthroughResumeRetainedProcess`, `TestSSHPassthroughResumeUnknownLiveness`, existing `TestResolveLaunchAuthToken` |
| .12 | `TestRemotePassthroughOldHelper`, `TestRemotePassthroughSafeStartupFailure` |
| .14 | `TestRemotePassthroughConfigOwnership`, `TestRemotePassthroughConfigSymlinks`, local ownership regressions |
| AC-EXECUTORS-SURVIVAL-005.3 | Existing standalone `manager_passthrough_scope_test.go` and `manager_startup_test.go` |

Decimal criteria use the `AC-EXECUTORS-SSH-PASSTHROUGH-001` prefix.

## E2E tests

Use new `apps/web/e2e/tests/ssh/passthrough.spec.ts` and
`apps/web/e2e/tests/ssh/mobile-passthrough.spec.ts` in `containers`.
The mobile file uses Pixel 5 device options and `.tap()` for keybar controls.
Verify that the context in `ssh-test-base.ts` inherits the device options. Exclude SSH files from
`mobile-chrome` so a Docker dependency cannot enter the ordinary mobile suite.

- No prepare script: remote output, accepted input, remote-only marker, MCP tool call (.1-.3, .5-.6).
- Reload/reconnect: same remote PID, bounded output, no duplicate initial prompt (.4, .6-.7).
- Interrupt, Stop, and Resume: remote descendants end, workspace remains, conversation resumes (.3, .9-.11).
- Siblings and uncertain transport: input isolation, no duplicate remote CLI, safe recovery (.7-.8, .11-.12).
- Phone Chat and keybar: remote response, resize, reconnect, interruption, and containment (.13).

## Work orders

- [x] [Task 01: Own remote passthrough in agentctl](task-01-remote-controller.md)
- [x] [Task 02: Launch and attach the SSH terminal](task-02-remote-launch-and-terminal.md)
- [x] [Task 03: Complete recovery and lifecycle parity](task-03-recovery-and-lifecycle.md)
- [x] [Task 04: Prove SSH desktop and phone flows](task-04-ssh-e2e-and-docs.md)

Execute sequentially: 01 -> 02 -> 03 -> 04. No delegation is authorized.

## Verification results

Diagnostic evidence only:

- `TestReproSSHPassthroughUsesHostFilesystem`: both expected-defect cases passed.
- `TestReproSSHPassthroughMCPBootstrap`: expected port failure and generic safe cause passed.
- `TestResolveLaunchAuthToken`: six existing cases passed.
- Temporary reproduction files removed. No live instance changed.

Design-package checks:

- `python3 scripts/list-docs.py validate`: passed, 383 decisions and 1,611 specifications.
- `python3 scripts/lint-spec-files.test.py`: all 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- PR-docs `validateCoverage`: actual documentation changes are exempt. A
  preflight with the planned lifecycle change validates all four work orders
  and their requirement/design references as covered, with no errors.

All four work orders are complete. Final managed SSH browser verification passed
all 14 tests in 4.3 minutes, including desktop and phone Stop/Resume, MCP scope,
backend interruption, sibling input isolation, readiness, and existing recovery.
The final local desktop terminal repeat passed all five tests in 33.9 seconds.
All 16 existing phone terminal cases and 79 focused web tests also passed.

Controller, process, config, client, lifecycle, gateway, executor, and adapter
race checks passed at their relevant implementation checkpoints. The final
Resume changes passed expanded lifecycle/executor/adapter race tests. Changed
Go lint reported zero issues. Web typecheck, targeted ESLint, and i18n checks
passed. Public-doc validation and its 63 tests passed. The specification catalog,
specification lint, and its 36 tests passed after the final spec status update.
Each work order records commands, results, and limits. Final documentation
coverage passed for actual changes and the lifecycle trigger, with no errors.
`git diff --check` passed at the initial implementation checkpoint.

### Documentation coverage preflight

Run from the repository root. Read Git's NUL-delimited output directly. Include
the linked documents needed by changed work orders. The second invocation also validates the lifecycle production trigger.

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const output = execFileSync('git', ['ls-files', '--modified', '--others', '--exclude-standard', '-z']);
const paths = [...new Set(output.toString('utf8').split('\0').filter(Boolean))];
const changedFiles = paths.map(filename => ({ filename, status: 'modified' }));
const documents = paths.filter(p => p.startsWith('docs/') && p.endsWith('.md'));
const required = [
  'docs/specs/executors/requirements/ssh-passthrough.md',
  'docs/specs/executors/requirements/agent-survival-session-state.md',
  'docs/specs/executors/system-design/ssh-passthrough.md',
  'docs/plans/ssh-passthrough/plan.md',
];
const fileContents = Object.fromEntries([...new Set([...documents, ...required])]
  .map(p => [p, fs.readFileSync(p, 'utf8')]));
const actual = validateCoverage({ changedFiles, fileContents });
const planned = validateCoverage({
  changedFiles: [...changedFiles, {
    filename: 'apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go',
    status: 'modified',
  }],
  fileContents,
});
console.log(JSON.stringify({ actual, planned }, null, 2));
if (!actual.ok || !planned.ok) process.exitCode = 1;
NODE
```

## Risks

- A PTY can exit before startup registration. Capture/replay correlated events
  from before start and test this race with channels.
- An uncertain start or prompt response can create duplicate work. Reconcile
  operation and submission identity. Do not automatically repeat uncertain writes.
- Shared project config needs cross-controller claims and conditional cleanup.
- Retained old helpers may not advertise passthrough. Preserve a safe recovery
  path without bypassing authentication or replacing a possibly live process.
- Actual reporter logs and live Proxmox/Tailscale reproduction are unavailable.
  Current evidence identifies deterministic backend defects independent of that setup.


## Second automated review remediation

Conflicting shared MCP updates now fail before writing. Replay uses bounded
pages, and event persistence no longer holds the client source guard needed by
cleanup. Regressions cover these failures and retained event identity checks.
The config, process, and lifecycle focused race tests passed. SSH terminal tests
now use explicit terminal locators and wait for the Resume response before UI
assertions. Browser delivery results and current CI/review status remain in the PR.


Cursor persistence also uses the runtime teardown guard and revalidates its
owner before saving. Successful and not-found SQLite reads cannot restore a
retired row or overwrite a replacement. The API ownership test covers identical
shared JSON, rejection, and first/last-owner cleanup.

The six affected backend packages have passing complete race results. One
gateway marker timeout passed unchanged in isolated and complete-package
repeats. SQLguard and store-conformance checks passed. Detailed commands and
results are recorded in Task 03.

Ready/running state writes and cursor writes now share the runtime teardown
guard. Publication occurs after releasing runtime and startup guards. The
12-case SQLite matrix and synchronous resume-binding/cleanup subscriber passed.
The complete lifecycle and gateway race suites passed after this extension.

Remote reset now resolves profile credentials before Stop and applies MCP
changes after a confirmed Stop. Preparation releases only the stopped current
generation's claims and rejects stale generations. Sibling claims remain
protected. Task 03 records the regression and verification results.
