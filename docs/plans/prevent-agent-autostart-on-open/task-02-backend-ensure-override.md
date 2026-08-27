---
id: "02-backend-ensure-override"
title: "Backend session.ensure auto_start override"
status: done
wave: 1
parallelism: sequential
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/prevent-agent-autostart-on-open.md"
---

# Task 02: Backend `session.ensure` `auto_start` override

## Acceptance

- `EnsureSessionOptions` gains `AutoStart *bool`; `EnsureSession` honors it:
  when `AutoStart` is `&false`, the session is created with
  `IntentPrepare` / source `created_prepare` even when the resolved workflow
  step has `auto_start_agent` in its on-enter actions. When `AutoStart` is nil
  or `&true`, the step-derived decision is unchanged.
- The prepare-only request is NEVER upgraded to an agent launch:
  `LaunchSessionRequest` gains a prepare-only marker (e.g. `NoAgentLaunch
  bool`), `EnsureSession` sets it when `AutoStart == &false`, and
  `shouldUpgradePassthroughPrepare` (`session_launch.go:172-174`) returns
  false when it is set. Without this, passthrough profiles are eagerly
  upgraded into `launchStart` and the agent starts anyway, violating the
  contract.
- The WS `session.ensure` handler parses an optional `auto_start` field and
  passes it through as the option.
- No behavior change for existing callers that never send `auto_start`.

## Verification

```bash
(cd apps/backend && go test ./internal/orchestrator/ -race -run 'TestEnsureSession|TestWsEnsureSession')
```

```bash
(cd apps/backend && go test ./internal/orchestrator/... ./internal/gateway/... -race)
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/session_ensure.go` (`EnsureSessionOptions` at `:26`, decision at `:87-94`)
- `apps/backend/internal/orchestrator/session_launch.go` (`LaunchSessionRequest` struct, `shouldUpgradePassthroughPrepare` at `:172-174`)
- `apps/backend/internal/orchestrator/handlers/handlers.go` (`wsEnsureSessionRequest` at `:100`, handler at `:104`)
- `apps/backend/internal/orchestrator/session_ensure_test.go` (or a new focused test file)
- `apps/backend/internal/orchestrator/session_launch_test.go` (passthrough upgrade regression)
- `apps/backend/internal/orchestrator/handlers/handlers_test.go`

## Dependencies

None (backend-only; the frontend sends the field from task 03).

## Inputs

- Spec "API surface → WebSocket `session.ensure`".
- `stepAllowsAutoStart` (`session_ensure.go:268`) and the existing
  `session_ensure_test.go` / `session_ensure_office_test.go` seeding patterns
  (a step whose on-enter has `auto_start_agent` → control case starts;
  override case prepares).
- `launchPrepare` / `shouldUpgradePassthroughPrepare` (`session_launch.go:148-174`):
  prepare requests for passthrough profiles are eagerly upgraded to
  `launchStart` unless `AutoStart` or `DeferredStart` is set; the new
  prepare-only marker must suppress that upgrade.

## Output Contract

`EnsureSession` accepts an explicit start-vs-prepare override; the WS contract
gains the optional `auto_start` field without breaking existing clients.
`auto_start: false` never starts an agent, including for passthrough profiles
(the `shouldUpgradePassthroughPrepare` upgrade is suppressed). Tests pin:
override-false prepares, override-absent starts, handler passthrough, and a
passthrough-profile regression (override-false still prepares).
