---
created: 2026-09-12
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-002
system_design:
  - ../../specs/integrations/system-design/github-authentication-02.md
legacy_specs: []
---

# Fix plan: Executor host GitHub CLI bridge

## Overview

[Issue #3072](https://github.com/kdlbs/kandev/issues/3072) requests HTTPS Git access through the host GitHub CLI in executor mode.
The issue is assigned to `carlosflorencio`.
The integration system owns this repair because it owns task credential selection.
The requirement and technical design lacked the optional HTTPS bridge.
This package adds that behavior to the existing authentication capability.

Implementation is complete. Tasks 01, 02, and 03 were executed in sequence in this session.
The production route, runtime propagation, regression coverage, and public guidance are included.
The review follow-up also closes source-aware indexed resolution, complete-environment
reconfiguration, generated-helper ownership, and profile credential-store selection.

## Confirmed root cause

Investigated repository head: `49df3794bc15d610988acc674637384ce8f1d4be`.
The canonical issue was open with no comments or image attachments during investigation.

In `executor_credentials.go`, `configureGitCredentialBrokerForRepositories` returns immediately after executor-mode managed-credential cleanup.
It never registers a host GitHub CLI helper.
An HTTPS origin therefore requires a separately configured Git helper, even when the CLI can supply a token.
Transport selection does not register that helper.

A temporary `TestRepro3072HostGHBridge` drove the real executor policy branch with a fake authenticated CLI.
A real `git credential fill` then failed with:

```text
fatal: could not read Username for 'https://github.com': terminal prompts disabled
```

The same test composed a hypothetical helper through `gitconfigenv.Merge`.
Credential lookup then succeeded, both inherited entries survived, and another host still failed.
The probe used fake credentials, isolated Git configuration, and no network.
The temporary test was removed after evidence collection.

## Scope

### In scope

- Optional host CLI credential availability probe and helper for Local/Worktree executor mode.
- Attached-host selection, explicit-token precedence, and managed-mode exclusion.
- Ordered indexed configuration across actual launch, resume, prepared-workspace, and child-process boundaries.
- Focused regression coverage and public explanation of the resulting behavior.

### Out of scope

- Global `gh auth setup-git`, user Git configuration writes, or agent prompt changes.
- SSH key creation, SSH/HTTPS remote rewriting, or additional repository permissions.
- New workspace automation bindings, broker changes, UI controls, migrations, or remote credential forwarding.
- Changes to sibling issues #3069, #3070, and #3071.

## Technical approach

The [requirement](../../specs/integrations/requirements/github-authentication.md) defines criteria 002.1 through 002.7.
The [design](../../specs/integrations/system-design/github-authentication-02.md#executor-host-cli-bridge) defines eligibility, lookup, composition, and runtime propagation.

Task 01 adds the optional route beside `configureGitCredentialBrokerForRepositories`.
Use a small injectable runner and `subproc.RunGH` with discarded command output.
Use the existing repository identity resolver and `gitconfigenv.Merge`.
Existing Git helpers remain earlier in the chain, so the new helper fills an authentication gap.

Task 02 closes the full-launch, resume, and prepared-workspace paths.
The strict resolver composes the indexed Git block by source after all managed definitions exist.
The lifecycle sends a complete composed snapshot through agentctl's replacement mode, while
request-only configuration keeps overlay semantics for ordinary callers.
It tests profile environment selection before probing and the final agentctl child environment.
Managed helper failure must never invoke the host CLI.
Task 03 documents the completed behavior and synchronizes results.

The existing task Git policy ADR already owns ordered composition and managed isolation.
No new ADR is needed for this implementation of executor-visible credential access.
The related clone-transport and task-terminal packages remain historical delivery records.
This package changes neither their completed scope nor their recorded test results.

## Tests

| Criteria | Planned test and file |
| --- | --- |
| 002.1, 002.2 | `TestExecutorHostGHBridgeCredentialFill` in `executor_host_gh_bridge_test.go`: real executor route, real Git, fake CLI. Initially fails because no helper exists. |
| 002.2 | `TestExecutorHostGHBridgePreservesIndexedConfig`: the supplied two-entry block, empty/absent block, duplicate helpers, malformed block, and combined count boundary. |
| 002.3 | `TestExecutorHostGHBridgeCredentialPrecedence`: request tokens, late profile tokens, enterprise tokens, existing user helper, and managed broker failure. |
| 002.4, 002.5 | `TestExecutorHostGHBridgeEligibility`: explicit local types, all remote types, mixed hosts/providers, unavailable CLI, timeout, and cancellation. |
| 002.6 | `TestExecutorHostGHBridgeLaunchResume` and `TestExecutorHostGHBridgePreparedWorkspace`: host availability changes, policy transition, and repeated preparation. |
| 002.2, 002.3, 002.7 | `TestCollectAgentEnvHostGHBridge` in agentctl `config_test.go`: inherited block plus task overlay, no global file changes, no secret output. |
| 002.3, 002.6 | `TestBuildEnvForExecutionHostGHBridge` in lifecycle `manager_launch_test.go`: late explicit token and environment replacement. |
| 002.2, 002.6 | `TestManagerConfigureReplacesOwnedHostHelperAndPreservesIndexedEnvironment` and `TestManagerConfigureWithEnvironmentReplacesCompleteIndexedBlock` in process `manager_temp_test.go`: overlay and complete replacement behavior for agent, tracker, and shell/process consumers. |
| 002.3, 002.5 | `TestExecutorHostGHBridgeProbeUsesProfileCredentialDirectories`, `TestExecutorHostGHBridgeResolvesEffectiveAgentAndExecutorProfiles`, and `TestLaunchPreparedSessionProbesEffectiveProfileCredentialStore`: profile HOME/GH_CONFIG_DIR selection through the production prepared-session entry point, request precedence, and no-token probe behavior. |

All executor test paths above are under `apps/backend/internal/orchestrator/executor/`.
Task 02 also covers the real subprocess helper under a login shell that replaces `PATH`.

## E2E tests

The end-to-end boundary is Git credential resolution in the effective task environment.
Use real `git credential fill` through the actual policy and environment composition paths with an isolated fake CLI.
This checks the user-visible authentication outcome without an artificial browser test.
No Playwright project or rendered UI change is required.
A live GitHub fetch/push is not part of automated validation and is not claimed by the reproduction.

## Work orders

- [x] [Task 01: Add the optional host CLI route](task-01-host-cli-route.md)
- [x] [Task 02: Complete runtime propagation](task-02-runtime-propagation.md)
- [x] [Task 03: Document executor HTTPS access](task-03-document-https-access.md)

## Verification results

Diagnostic evidence:

- `go test -tags fts5 ./internal/orchestrator/executor -run '^TestRepro3072HostGHBridge$' -count=1 -v`: passed.
  The test asserted the current failure and the successful hypothetical helper composition.
- `go test ./internal/gitconfigenv -count=1`: passed.
- GitHub assignment: re-read issue assignees and confirmed `carlosflorencio`.

Design validation:

- `python3 scripts/list-docs.py validate`: passed (264 decisions, 818 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/executor-host-gh-bridge`: passed.
- Work-order requirement IDs, acceptance IDs, relative links, and existing source paths: checked.
- `git status --short -- docs/plans/executor-host-gh-bridge`: confirmed the new package is present.
Implementation validation:

- The final affected-package run passed with `-count=1` for `internal/agent/runtime/agentctl`,
  `internal/agent/runtime/environment`, `internal/agent/runtime/lifecycle`,
  `internal/agentctl/server/api`, `internal/agentctl/server/config`,
  `internal/agentctl/server/process`, `internal/backendapp`, `internal/gitconfigenv`,
  and `internal/orchestrator/executor`.
- `make -C apps/backend lint` passed with zero issues on the final tree.
- `make -C apps/backend build` passed for the host and remote runtime binaries.
- The strict production-shaped test `TestBuildEnvForExecutionHostGHBridge_ComposesStrictProfileBlocks`
  composes agent-profile, executor-profile, and managed Git blocks while preserving each indexed entry.
- `TestConfigureAndStartAgentReplacesComposedRuntimeEnvironment` verifies that lifecycle sends one
  composed snapshot with `replace_env: true`; process-manager tests cover reconfiguration, tracker,
  one-shot, shell/process propagation, user hooks/notes, and removal of obsolete marker-owned helpers.
- `TestLaunchPreparedSessionProbesEffectiveProfileCredentialStore` exercises HOME and GH_CONFIG_DIR
  mismatches through the production prepared-session entry point. The profile tests also cover both
  profile sources, request precedence, unavailable replacement behavior, and no token output.
- `node --test scripts/validate-public-docs.test.mjs` passed (62 tests), and
  `node scripts/validate-public-docs.mjs` validated 46 published pages.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`,
  `git diff --check`, and changed-file `gofmt` checks passed.
- An earlier full backend-suite run passed all changed packages but returned nonzero for existing
  process-tree probe tests in `internal/agentctl/server/process/probe` and office FTS migration
  tests in `internal/office/repository/sqlite`; the final validation above is the current scoped run.

## Risks

- CLI token availability does not prove repository permission or network reachability.
- A backend service can have a different home directory or credential store from the user's terminal.
- Later profile variables and reused executor environments can change credential precedence.
- Shell startup can replace `PATH`. The helper must resolve the intended host executable safely.
- Existing user helpers can return stale credentials before the optional fallback. This repair preserves their precedence.

## External references

- [Git configuration protocol](https://git-scm.com/docs/git-config#Documentation/git-config.txt-GIT_CONFIG_COUNT): indexed entries and runtime configuration.
- [Git credential helper behavior](https://git-scm.com/docs/gitcredentials): helper ordering and URL contexts.
- [GitHub CLI token command](https://cli.github.com/manual/gh_auth_token): host-specific token retrieval.

- [GitHub CLI environment precedence](https://cli.github.com/manual/gh_help_environment): explicit tokens and CLI configuration directory.
