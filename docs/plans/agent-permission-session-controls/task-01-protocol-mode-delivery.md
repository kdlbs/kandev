---
id: "01-protocol-mode-delivery"
title: "Apply permission modes through ACP"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-007
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.1
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.3
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.7
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.16
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.1
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.2
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.3
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.4
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.8
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Apply permission modes through ACP

## Outcome and scope

Replace the settings overlay with the protocol flow in the system design.
Add failing regression tests before implementation.

- Prefer the advertised mode config option and use its returned current value.
  Cover actual IDs, grouped choices, clamps, missing response values, and errors.
- Keep legacy mode support. Accept mode values in config-option notifications.
  Test the Claude 0.81.2 shape without a current-mode notification.
- Serialize both mode entry points. Preserve stale-session and timeout guards.
  Do not invent a current value from the requested value.
- Resolve and apply the winning start mode before prompt dispatch. Propagate an
  unmet explicit mode. Preserve no-mode behavior, resume, and context reset.
- Remove mode-specific settings writes, uploads, and directory redirection from
  Docker, remote Docker, SSH, Kubernetes, and agent declarations.
- Preserve explicit bundle transfer, authentication, and verified container-only
  process availability controls. Keep these independent of mode-file delivery.

## Exclusions

No shared-settings fallback, new profile fields, provider forks, unrelated mode
attribution redesign, or changes to provider permission classification.

## Likely files

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_mode_state.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/initial_mode*.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_*.go`
- `apps/backend/internal/agent/agents/initial_mode.go` and `claude_acp.go`

## Acceptance

1. Both protocol paths return authoritative mode results without settings mutations.
2. An explicit mode settles before the first prompt, including reset and resume.
3. Executor authentication and selected bundle behavior remain intact.

## Verification

From the repository root:

```bash
cd apps/backend
go test ./internal/agentctl/server/adapter/transport/acp -count=1
go test ./internal/agent/runtime/lifecycle ./internal/agent/agents -count=1
```

Tests must include file-content assertions and launch-environment assertions.
Use two concurrent sessions and verify that each retains its own selected mode.
Use deterministic request barriers for ordering tests.

## Results

Implemented. The ACP adapter prefers the advertised `mode` config option,
selects its actual option ID and grouped value, and uses returned config options
as authoritative evidence. It keeps legacy `session/set_mode`, accepts the
Claude 0.81.2 `config_option_update` shape, serializes both entry points, and
rejects stale, missing, or mismatched observations without substituting the
requested value.

Lifecycle applies the winning explicit mode before prompts on new, resumed,
and reset sessions. An unavailable or unconfirmed mode holds the prompt; absence
of an explicit mode stays unchanged. Automatic settings overlays and mode-based
configuration-directory redirection were removed. Selected portable settings
and credentials remain on their existing transfer paths, and `IS_SANDBOX` is
limited to container availability.

Validation passed:

- `go test ./internal/agentctl/server/adapter/transport/acp -count=1`
- `go test ./internal/agent/runtime/lifecycle ./internal/agent/agents -count=1`
- Focused lifecycle start, resume, reset, container, and settings-isolation tests.
- `go test -race ./internal/agentctl/server/adapter/transport/acp -run 'Test(SetMode|SetConfigOptionMode|ConcurrentACPAdaptersKeepSessionModesIsolated|ConcurrentSetModeRequestsCannotShareAReport)' -count=1 -timeout=180s`

The implementation is integrated with the later contributor commits in a separate
delivery checkout. The original shared implementation checkout remains intact.
Exact delivery and CI evidence is recorded in the Kandev task plan.

Follow-up regressions also pass for mode versus model and effort snapshots in
both request orders, unrelated config responses preserving unresolved mode
uncertainty, mode-only config catalogs on new/load/reset, unsolicited config
updates publishing a mode event, and stale-session config updates being
ignored.

Verification on the reconciled local tree passed:

- `go test ./internal/agentctl/server/process ./internal/task/models ./internal/agent/agents -count=1 -timeout=300s`
- `go test ./internal/agent/runtime/lifecycle ./internal/agentctl/server/adapter/transport/acp -count=1 -timeout=300s`
- `go test ./internal/orchestrator -run '^TestHandleSessionModeEvent$' -count=1 -timeout=120s`
- The focused ACP mode/config race run passed before the latest-main merge.

Follow-up review cancellation finding is fixed. Full config changes, including
mode, model, other config options, and new/load/reset session operations, now
acquire the shared snapshot gate with a context-aware wait. A deterministic
regression holds a model RPC, then cancels waiting mode, new-session,
load-session, and reset-session operations. Each returns before the provider is
released, sends no follow-up RPC, leaves the active session cache unchanged,
and releases mode or transition ownership. The existing RPC-cancellation
tracker test now uses an active caller context and a provider-returned
`context.Canceled` error, separate from the canceled-waiter case.

Latest verification passed:

- `go test ./internal/agentctl/server/adapter/transport/acp -count=1`
- `go test -race ./internal/agentctl/server/adapter/transport/acp -run '^(TestConfigChangeWaitersRespectCancellationAndDoNotMutateSession|TestModeAndModelConfigSnapshotsShareOrdering|TestModeAndOtherConfigSnapshotsShareOrdering)$' -count=1 -timeout=180s`

Delivery cleanup removes unused overlay readers and the unused mode observer.
The mode operation now separates capability selection from config RPC handling.
Model changes read cached configuration only after acquiring the shared gate.
Ordering tests live in a separate file. These changes preserve behavior, and
changed-package Go lint passes with no issues.
