---
id: "01-restore-runtime-configuration"
title: "Restore the complete runtime configuration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/acp-model-configuration-summary.md"
---

# Task 01: Restore the Complete Runtime Configuration

## Acceptance

- Both context-reset paths capture one immutable effective `SessionRuntimeConfig` before provider defaults can publish.
- Both paths restore the model, permission mode, and each non-model configuration option in deterministic order.
- If a captured field is rejected, Kandev reports an error and publishes no ready or reset-success event.
- The lifecycle does not expose the fresh session as ready until every captured field is restored.

## Verification

```bash
cd apps/backend && go test -race -run 'TestManager_(ResetAgentContext|RestartAgentProcess)_(ReappliesSessionRuntimeConfig|FailsClosedOnRuntimeConfigRestore)$' ./internal/agent/runtime/lifecycle
```

Write the regression tests first. The fast-path test must fail because current code restores the fresh default mode and omits configuration options.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`

## Dependencies

None.

## Parallelism

Sequential. This task owns the backend behavior required by every later task.

## Inputs

- Reset behavior in the linked spec
- `docs/decisions/2026-08-18-context-reset-preserves-runtime-configuration.md`
- Plan sections `Capture one effective runtime configuration`, `Restore one effective runtime configuration`, and `Regression fixture`
- `models.SessionRuntimeConfig` and `effectiveSessionRuntimeConfig`
- Existing model policy, session initialization layers, and workspace-rebind option filtering

## Risks

- Do not let provider-default events mutate the captured map.
- Do not send model or mode values again through `SetConfigOption`.
- Do not fabricate cache state after a rejected provider operation.
- Preserve `RuntimeConfigOptionsSet` semantics for an authoritative empty option set.

## Output contract

Report the RED result, the configuration precedence, the apply order, changed files, and exact test results. Update this task and `plan.md` in the same conversation.

## Results

RED: the focused lifecycle command failed all four new regression tests before
the implementation. The fast path omitted configuration-option requests and
the restart path did not restore the captured model/options; both rejection
cases incorrectly returned success.

The effective snapshot starts with profile values, applies persisted session
runtime values and explicit overrides through `WorkspaceInfo`, then falls back
to the live model and mode caches when a persisted field is absent. The option
map is cloned before reset or restart. Restoration order is model, permission
mode, then non-model options sorted by option ID. A rejected mode or option
marks the execution failed and keeps `AgentBootReady` and
`AgentContextReset` unpublished until all fields succeed.

Changed files:

- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_profile.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_runtime_config_reset_test.go`

Verification:

- `cd apps/backend && go test -race -run 'TestManager_(ResetAgentContext|RestartAgentProcess)_(ReappliesSessionRuntimeConfig|FailsClosedOnRuntimeConfigRestore)$' ./internal/agent/runtime/lifecycle` — 4 passed.
- `cd apps/backend && go test -race ./internal/agent/runtime/lifecycle` — 1,911 passed.

PR fixup coverage added after review:

- Live `ConfigOptions` snapshots are authoritative when available, so removed
  persisted options are not restored and newer provider values win over stale
  profile values.
- Option-only and explicitly settled empty catalogs satisfy the fresh-session
  readiness gate without waiting for a model list.
- The mock can reject model restoration, and the reset test verifies that mode,
  options, ready events, and reset-success events are not applied or published.
- Reset failures persist the failed executor-running status before returning.

Additional verification:

- Focused fixup tests — 4 passed.
- `cd apps/web && pnpm e2e:run tests/workflow/workflow-step-proceed.spec.ts -- --grep "preserves session settings across context reset"` — 1 passed.
- `cd apps/backend && make lint` — 0 issues.
