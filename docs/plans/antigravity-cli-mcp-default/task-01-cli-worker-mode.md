---
id: "01-cli-worker-mode"
title: "Enable HTTP MCP by default for the Antigravity CLI"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-003
  - REQ-AGENTS-ANTIGRAVITY-ACP-004
  - REQ-AGENTS-ANTIGRAVITY-ACP-006
  - REQ-AGENTS-ANTIGRAVITY-ACP-009
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
  - REQ-AGENTS-ANTIGRAVITY-ACP-011
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-003.5
  - AC-AGENTS-ANTIGRAVITY-ACP-003.14
  - AC-AGENTS-ANTIGRAVITY-ACP-003.15
  - AC-AGENTS-ANTIGRAVITY-ACP-004.11
  - AC-AGENTS-ANTIGRAVITY-ACP-006.2
  - AC-AGENTS-ANTIGRAVITY-ACP-006.5
  - AC-AGENTS-ANTIGRAVITY-ACP-009.1
  - AC-AGENTS-ANTIGRAVITY-ACP-010.7
  - AC-AGENTS-ANTIGRAVITY-ACP-011.3
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 01 Enable HTTP MCP by default for the Antigravity CLI

## Summary

Start the selected `agy acp` process in worker mode for local execution. Let the real ACP response supply the HTTP capability for required MCP servers.

## In scope

- Check `agy acp --help` after the version check. Match the exact worker option. Reuse the bounded output runner and honor caller cancellation.
- Include the flag in the resolved CLI command. Share the Antigravity argument rule at the task, preview, and utility composition points before wrappers apply.
- Keep the enabling flag effective after profile options, including explicit false values. Preserve other argument values and any `--` delimiter.
- Keep the utility allowlist tied to the qualified executable and base arguments. Construct the existing CLI login command from the qualified executable.
- Update the Antigravity section of the public agent guide with the qualification rule and automatic worker flag.

## Exclusions

- MCP routes, transport overrides, debugger output formatting, and server provisioning.
- Profile migrations, model catalog changes, and native credential changes.

## Acceptance

1. Fresh local and worktree processes use worker mode for every selected model. An existing enabling or disabling profile entry leaves worker mode enabled.
2. Previews and probes use the effective worker command. Sign-in uses `agy acp --login`. An unsupported CLI follows the existing new-work fallback.
3. Core reaches `session/new` with advertised HTTP. Restore preserves the native CLI ID and variant. Stop reaps the worker and preserves independent processes.

## Verification

First extend `TestAntigravityLocalLaunchUsesCLIAndPreservesProfileArgs` to require `--process-per-session` with a profile that omits it. The current command fails this assertion.

- Extend `TestResolveLocalRuntimeSelection` for a CLI with the flag, a CLI without it, and a cancelled help check. Extend the saved-runtime tests for an unavailable worker flag and preserved variant selection.
- Cover value-aware argument placement when a literal `--` or a worker-looking token is the value of `--log-file`, and preserve a real delimiter.
- Require successful help output with an exact option declaration. Cover failed help containing the declaration and successful prose-only output for both standalone fallback and saved CLI failure.
- Extend the selected-runtime preview, login, and utility tests for the new command. Cover existing true and false spellings with a profile that also has unrelated quoted options.
- Add `TestAntigravityCLIWorkerModeDeliversCoreMCP` with a launch and ACP fixture. Exercise two model selections. Assert that HTTP is advertised and that a required Core server reaches session creation.
- Exercise a live child process through `session/new`, stop one of two independent launches, and verify only its child exits. Start a fresh saved-CLI launch, call `session/resume` with the stored native ID, and verify ID continuity and child cleanup.
- Extend `TestAntigravityTaskResumeCommandUsesSavedRuntimeAfterRestart` for the worker command. Record the native ID and process cleanup in the isolated smoke result.

```bash
cd apps/backend
go test -trimpath ./internal/agent/antigravityruntime ./internal/agent/agents \
  ./internal/agent/runtime/lifecycle ./internal/agent/settings/controller \
  ./internal/agent/hostutility ./internal/agent/loginpty \
  ./internal/agentctl/server/utility ./internal/agentctl/server/adapter/transport/acp \
  -run 'Antigravity|TestResolveLocalRuntimeSelection|TestResolveSavedRuntime|TestResolveLocalHonorsCancellation|TestRequireWorkerModePreservesOptionValuesAndDelimiter|TestHasWorkerModeFlagRequiresExactOption|TestResolveLocalFallsBackWhenWorkerHelpIsUnqualified' -count=1
```

From the repository root, run the existing public-docs checks.

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Use the [isolated instance procedure](../../../.agents/skills/debug/references/instance.md#launch-isolated-instance) for the runtime smoke. Reopen a saved CLI conversation. Verify Core discovery, native ID continuity, and worker cleanup through the task flow.

## Files likely touched

- `apps/backend/internal/agent/antigravityruntime/local.go`, `resolve.go`, and `local_test.go`
- `apps/backend/internal/agent/agents/antigravity_acp.go` and `apps/backend/internal/agent/loginpty/antigravity_login_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go` and its Antigravity launch and resume tests
- `apps/backend/internal/agent/settings/controller/agent_config.go` and its Antigravity preview test
- `apps/backend/internal/agentctl/server/utility/launch_wrapper.go` and the Antigravity utility probe tests
- `docs/public/agents-and-profiles.md`

## Dependencies

Use the resolver and saved variant binding from the [Antigravity CLI ACP package](../antigravity-cli-acp/plan.md).

## Risks

The installed CLI can change between launches. Check the selected executable at each resolution. Keep its qualified command for the current launch attempt.

## Results

Qualified local CLI selection by ACP version and `agy acp --help` worker-option support. New local work falls back to the standalone runtime when the CLI does not qualify. Saved CLI conversations keep their recorded runtime and surface a typed resume error if the CLI no longer qualifies.

The resolved command carries `--process-per-session`. Lifecycle launches, saved resumes, previews, and utility probes enforce the flag after profile arguments and before `--`. CLI sign-in remains `agy acp --login`. Public guidance now documents qualification and automatic worker mode in the root README and agent guide.

The two-model ACP fixture passed. It launched the selected process command, received HTTP capability advertisement, and observed the required Core HTTP server in both `session/new` requests. The lifecycle fixture also passed with live child processes: stopping the first of two launches reaped its child while the second child remained alive; after a fresh manager selected the saved CLI runtime, `session/resume` received the stored native session ID, the adapter retained that ID, and stopping the resumed manager reaped only its child. Final teardown results are asserted.

Resolver regressions passed for `--log-file` values equal to `--` and `--process-per-session`, a real argument delimiter, and explicit false worker settings. CLI qualification passed only for a successful `Flags:`/`Options:` declaration. Failed help output containing the option and successful prose-only help both select the standalone runtime for new work and return a typed saved-CLI error for resume. Existing coverage also passed for caller cancellation, saved runtime selection, preview, utility admission, and login command shape.

Verification passed:

- The exact package-filtered Go command in this work order, including the argument, qualification, child reaping, and saved native resume regressions.
- `make -C apps/backend build`.
- `node --test scripts/validate-public-docs.test.mjs` (63 tests).
- `node scripts/validate-public-docs.mjs` (56 published pages).
- `gofmt -l` on all changed Go files and `git diff --check`.

The isolated Kandev task smoke reached ACP startup but stopped before native session creation because the isolated home had no Google sign-in. No credentials were copied. The isolated instance and temporary workspace were removed. The deterministic ACP fixture now covers `session/new`, process cleanup, restart-time `session/resume`, and native ID continuity without requiring credentials. The work order remains in progress until the isolated task flow can create and resume a real native session.

Changed-package lint first found two capitalized error strings. Both are corrected. Later lint attempts could not start because separate `golangci-lint` processes on unrelated packages were active. The package tests and backend build passed after the corrections.
