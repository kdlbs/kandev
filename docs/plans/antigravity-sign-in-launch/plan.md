---
created: 2026-11-04
status: done
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-003
  - REQ-AGENTS-ANTIGRAVITY-ACP-009
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
legacy_specs: []
---

# Fix plan: Antigravity sign-in launch

## Overview

Repair the command sent to agentctl when an operator starts Antigravity sign-in from Settings.
Keep the executable in the structured argument array, then test the actual configure HTTP boundary.
One work order owns the caller correction, regression tests, and delivery-record updates.

Agents owns this repair because it owns the provider runtime and authentication contract.
Reuse the [existing requirements](../../specs/agents/requirements/antigravity-acp-agent.md) without adding acceptance criteria.
Clarify the existing [system design](../../specs/agents/system-design/antigravity-acp-agent.md#explicit-local-sign-in).
This is an implementation defect against the existing criteria, not a new authentication flow.
The requirement and design documents remain draft while the wider package lacks native qualification.

## Confirmed root cause and evidence

On November 4, the operator installed Antigravity from `/settings/agents/browse`, then started sign-in from the profiles page.
The host log records installation success at 20:18:29 UTC.
The sign-in request at 20:18:50 UTC fails during configuration, before native startup or Google authentication.

`configureAntigravityLoginInstance` resolves the executable correctly, then calls `ConfigureAgent` with `args[1:]` as `agentArgs`.
The client serializes that array as `agent_args`.
`process.Manager.configure` treats a present array as the complete command, with precedence over the separate `command` string.
On Linux, the received array is `["--uid="]`.
`config.ValidateCommandArgs` rejects the flag as an executable and produces the reported HTTP 500.
An executable-only command instead produces a present empty array, which also fails validation.

The running build was `067632acaa29e657c45162c2b8eb375958497300`.
The workspace baseline was `79b5b607d7cde61ca4ca17fbf3de4541ec26d19d`.
Both contain the same incorrect caller and receiver contract.
A temporary test exercised the real sign-in caller and HTTP client against the production argument validator.
It reproduced the exact error and accepted the complete command array. The temporary test was removed.
The permanent regression must use the actual configure handler, so it also tests request decoding and argument precedence.

The retained baseline bundle is `.kandev/diagnostics/dc8faf4539fdb2aec09090bb41f58909.zip`.
Its manifest reports truncation of older logs; the relevant November 4 install and sign-in events are retained.
The manifest identifies the running build above. Do not commit the bundle or raw runtime logs.

## Installer findings

The Settings action uses `AntigravityACP.LocalInstall`, through `Controller.EnqueueInstall`, rather than a shell install script.
It installs reviewed ACP 1.4.5 from Google into the Kandev managed cache.
Both required files are present and executable on the affected Linux host:

- `agy_acp_server.par`: the standalone ACP server.
- `localharness_external`: its native companion.

The resolver supplies the companion through `ANTIGRAVITY_HARNESS_PATH`.
The installer validates both files and publishes only a complete bundle.
It does not install the `agy` CLI or a separate npm bridge.
An existing `/root/.local/bin/agy` predates the November 4 bundle installation.
The current standalone integration does not require that CLI.

The [ACP registry entry](https://github.com/agentclientprotocol/registry/blob/main/antigravity-acp/agent.json) still describes the pinned standalone distribution.
[Google also documents `agy acp`](https://antigravity.google/docs/ide/extensions/acp/) as a CLI-based ACP route.
This repair does not migrate runtimes, change credentials, or infer compatibility from that alternative route.

## Scope

### In scope

- Preserve the resolved executable and every argument in the sign-in configure request.
- Add a regression through the real agentctl configure handler and client.
- Cover Linux flags, executable-only commands, and executable paths that contain spaces.
- Run the existing installer, resolver, and structured-command contract tests.
- Link the repair into the original delivery package without closing unrelated gates.

### Out of scope

- Installing `agy`, adding a bridge package, changing artifact pins, or modifying the installer.
- Migrating to `agy acp`, importing CLI credentials, or changing OAuth ownership.
- Changing the generic configure API, argument validation, launch wrappers, or other provider callers.
- UI markup, localization, responsive composition, and new browser test infrastructure.
- Native SIGILL remediation, CPU affinity, real Google consent, and subscription inference qualification.

## Technical approach

### Complete structured command

In `apps/backend/internal/agent/hostutility/antigravity_login.go`, retain the existing resolution, empty-command guard, capability check, and environment merge.
Pass the full `args` slice to `Client.ConfigureAgent` as `agentArgs`.
The existing first argument can remain the executable string because structured arguments take precedence.
Do not rebuild argv with string splitting or invoke a shell.
Do not prepend the executable inside the generic client: its other callers already provide full argv.

### Regression at the HTTP boundary

Add `antigravity_login_command_test.go` beside the sign-in owner.
Use `httptest`, the real `agentctl.Client`, `api.NewServer(...).Router()`, and `process.Manager` with temporary configuration.
Supply deterministic local runtime commands through a test implementation of `agents.LocalRuntimeAgent`.
Embed the real Antigravity declaration to preserve its inference and runtime metadata.
No test requires a vendor executable, installed CLI, account, credential store, or external network.

Advertise `antigravity_personal_auth_v1` on the test identity response.
Route configure requests through the actual API handler.
Intercept start with an explicit test marker after configuration, before any child process or stream starts.
Assert the start marker and the exact receiver `InstanceConfig.AgentArgs`.
The test must fail on the old caller because configure rejects the stripped array before it reaches start.
Do not accept an arbitrary later error as evidence of success.
Preserve the paired harness environment value in the received configuration.
Register test cleanup immediately and use bounded contexts and channel synchronization.

### Compatibility matrix

| Command shape | Required receiver argv | Evidence and failure behavior |
| --- | --- | --- |
| Linux standalone server | Absolute executable, then `--uid=` | New regression reaches controlled start. A flag-only array remains invalid. |
| Executable-only server | One executable token | New regression retains that token; a present empty array remains invalid. |
| Path containing spaces | One intact executable token, then unchanged arguments | New regression prevents command splitting. |
| Windows-style command | Executable token with backslashes, plus unchanged arguments | Platform-shaped input tests serialization without executing a foreign binary. |
| Other providers and legacy command fallback | Existing full-array or absent-array behavior | Existing client/API contract tests retain precedence and rejection rules. |

## Tests

All test additions below were delivered by [Task 01](task-01-preserve-sign-in-command.md).

| Criteria | Evidence |
| --- | --- |
| AC-AGENTS-ANTIGRAVITY-ACP-003.1, 003.2, 003.5 | `TestAntigravityLoginConfigurePreservesCommand` in `antigravity_login_command_test.go`, with platform-shaped command cases. |
| AC-AGENTS-ANTIGRAVITY-ACP-009.1, 009.2, 009.3 | Existing `TestInstallAntigravityBundle`, `TestResolveAntigravityBundle`, and `TestAntigravityHostLaunchPolicy`. No installer change. |
| AC-AGENTS-ANTIGRAVITY-ACP-010.2, 010.4 | New caller-to-real-configure-handler regression preserves executable, arguments, and harness environment before controlled start. |
| Existing structured-command contract | `TestConfigureAgent_SerializesStructuredArgsAndOmitsAbsentFallback`, `TestHandleAgentConfigure_PrefersPresentStructuredArgs`, and `TestHandleAgentConfigure_UsesLegacyOnlyWhenArgsAbsent`. |

Run the exact commands in [Task 01](task-01-preserve-sign-in-command.md#verification).
The recorded red result came before the correction and the green result after it.
The regression is not a test of the validator alone.

## E2E tests

This backend-only repair does not change rendered UI or require an artificial browser test.
Its regression spans the sign-in owner, real client serialization, actual HTTP handler, and process configuration.
It deliberately stops before native execution and does not prove Google consent or model readiness.

The original [Task 03](../antigravity-local-subscription/task-03-google-sign-in.md) still owns the broader browser flows:

| Existing acceptance coverage | Planned original suite | Project |
| --- | --- | --- |
| AC-AGENTS-ANTIGRAVITY-ACP-010.4 through 010.6; 015.1 through 015.3 | `apps/web/e2e/tests/settings/antigravity-setup.spec.ts` | `chromium` |
| Same setup outcomes on phones | `apps/web/e2e/tests/settings/mobile-antigravity-setup.spec.ts` | `mobile-chrome` |

Those two specs are absent at this baseline and remain open in the original package.
Do not mark them passed from this regression or create partial replacements under new names.
Original [Task 07](../antigravity-local-subscription/task-07-native-qualification.md) retains live native and Google qualification.

## Companion delivery records and public docs

The original [plan](../antigravity-local-subscription/plan.md) remains `partial`.
Its Task 03 remains `in_progress`, and Task 07 remains `blocked`.
Those records link this repair and keep historical evidence distinct from the new regression.
The broader requirement and system design remain draft.

Public setup documentation currently describes manual-only installation.
Its correction remains in original Task 07, which gates setup publication on native qualification.
This backend-only repair package changes no public commands or UI terminology.
It does not claim that the CLI-based route or real subscription flow has been qualified.

## Work orders

- [x] [Task 01: Preserve the complete sign-in command](task-01-preserve-sign-in-command.md)

Execution was sequential in the primary session.
The work order implemented the caller correction and its regression; the companion package retains its own browser and native qualification gates.

## Verification results

Planning checks passed on November 4:

- `python3 scripts/list-docs.py validate`: 431 decisions and 1915 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- Local Markdown file-link checks: all targets exist.
- `.github/scripts/pr-docs.cjs` coverage preflight: the documentation-only diff is exempt.
  A simulated change to the sign-in source is covered by the linked work orders, with no reference errors.
- `git diff --check -- docs/specs/agents docs/plans/antigravity-sign-in-launch docs/plans/antigravity-local-subscription`: passed.

Implementation and the permanent regression: complete.
On November 4, `configureAntigravityLoginInstance` now passes the complete `command.Args()` slice to `Client.ConfigureAgent`.
`antigravity_login_command_test.go` drives the sign-in caller through the real agentctl client, the production configure handler and `process.Manager` receiver, and a controlled start marker.
The temporary test is gone; run the permanent regression with Task 01's commands.

Regression evidence:

- Red before the correction: all four table cases failed at configure with the reported receiver errors, including `agent command executable is invalid` for `--uid=` and `agent command cannot be empty` for the executable-only command.
- Green after the correction: all four cases reached the controlled start marker, and the receiver kept the complete argv plus `ANTIGRAVITY_HARNESS_PATH`.
- Existing client, API, installer, resolver, and host launch-policy tests passed unchanged.

Implementation checks passed on November 4:

- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/hostutility -run '^TestAntigravityLoginConfigurePreservesCommand$' -count=1 -v)`: red before, green after.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/hostutility -run '^TestAntigravity' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/agentctl ./internal/agentctl/server/api -run '^Test(ConfigureAgent_|HandleAgentConfigure_)' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/antigravityruntime -run '^Test(InstallAntigravityBundle|ResolveAntigravityBundle)$' -count=1)`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/agent/hostutility/antigravity_login.go apps/backend/internal/agent/hostutility/antigravity_login_command_test.go`: 0 issues.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, the coverage preflight, and `git diff --check`: passed.

Prior diagnosis reproduced the reported error in an isolated temporary Go test.
The installer and resolver tests also passed during the earlier installation check.
Neither result is live native qualification or a real Google sign-in.

## Risks

- Removing the validation blocker can expose a later native startup or OAuth failure.
- Original Task 07 records a Linux SIGILL. This diagnosis did not rerun the vendor binary or resolve that gate.
- A fake configure receiver that ignores structured arguments would miss this regression; use the actual HTTP handler.
- An executable-only command needs a nonempty full array on every host, even when no flags follow it.
- Successful configure/start-boundary evidence does not establish authentication, model entitlement, or callback reachability.
