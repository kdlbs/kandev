---
id: "02-custom-acp-session-restore"
title: "Restore an operator-registered ACP agent's session on reconnect"
status: done
wave: 2
depends_on:
  - "01-operator-registered-acp-agents"
plan: "plan.md"
requirements:
  - REQ-AGENTS-CUSTOM-ACP-002
acceptance_criteria:
  - AC-AGENTS-CUSTOM-ACP-002.1
  - AC-AGENTS-CUSTOM-ACP-002.2
  - AC-AGENTS-CUSTOM-ACP-002.3
  - AC-AGENTS-CUSTOM-ACP-002.4
system_design:
  - ../../specs/agents/system-design/custom-acp-agents.md
---

# Task 02: Restore the provider session on reconnect

## Summary

`CustomACPAgent.Runtime()` declares `SessionConfig.NativeSessionResume = true`, so a reconnect of an
operator-registered ACP agent sends the stored provider session ID through the adapter's existing
`session/resume` / `session/load` negotiation instead of always sending `session/new`.

## In scope

- `internal/agent/agents/custom_acp.go`: set `SessionConfig{NativeSessionResume: true}` in
  `Runtime()`.
- A unit test in `internal/agent/agents/custom_acp_test.go` for the declared gate.
- Lifecycle tests in `internal/agent/runtime/lifecycle/` that drive `InitializeSession` with a real
  `agents.NewCustomACPAgent(...)` against the existing mock agentctl WebSocket server
  (`newMockAgentServer`): a stored ID produces exactly one `agent.session.load` carrying that ID and
  no `agent.session.new`; the canonical capability-mismatch error produces one load and one new; an
  unrecognized load error produces no `agent.session.new` and returns the error.
- An E2E spec, `apps/web/e2e/tests/session/custom-acp-session-restore.spec.ts`, that registers an
  ACP custom agent whose command is the mock-agent binary, completes one turn, restarts the backend,
  resumes, and asserts the session's stored provider session ID is unchanged.
- The system README or scoped `AGENTS.md` only if a statement there becomes inaccurate.

## Out of scope

- Any change to the lifecycle failure classifier, the adapter capability gate, or the restore `cwd`.
- A per-definition restore setting or a UI change.
- Terminal (passthrough) custom agents.

## Acceptance

1. **AC-AGENTS-CUSTOM-ACP-002.1 / .4:** a custom ACP agent built from a definition with no new
   fields reports `NativeSessionResume = true`, and `InitializeSession` with a stored ID sends
   `agent.session.load` for that exact ID and never `agent.session.new`.
2. **AC-AGENTS-CUSTOM-ACP-002.2 / .3:** with the capability-mismatch error the launch succeeds on a
   replacement session; with an unrecognized error the launch fails and no replacement is created.
3. The E2E spec shows the same provider session ID before and after a backend restart.

## Verification

```bash
cd apps/backend && go test ./internal/agent/agents/ -run 'CustomACP' -count=1
cd apps/backend && go test ./internal/agent/runtime/lifecycle/ -run 'CustomACP' -count=1
make -C apps/backend lint
cd apps/web && pnpm e2e:run tests/session/custom-acp-session-restore.spec.ts
```

On Windows the `.sh` runners do not run. After `make -C apps/backend build e2e-plugin-package`
and `pnpm run build:e2e`, call Playwright directly:
`KANDEV_E2E_SKIP_FRESHNESS=1 pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium --workers=1 custom-acp-session-restore`.

## Likely files

- `apps/backend/internal/agent/agents/custom_acp.go`
- `apps/backend/internal/agent/agents/custom_acp_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/custom_acp_session_restore_test.go` (new)
- `apps/web/e2e/tests/session/custom-acp-session-restore.spec.ts` (new)

## Risks

- A CLI that answers an unknown session ID with an unrecognized error shape now surfaces a recovery
  failure with a **Start fresh** action where it previously got a silent new conversation. This is
  the existing contract for built-in agents (REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001).
- The E2E spec resolves `mock-agent` through the E2E `PATH` and creates its own profile with the
  mock's `mock-fast` model, so it does not depend on the capability probe having seeded a model.

## Results

Red before the change, green after it:

- Unit: `TestCustomACPAgentDeclaresNativeSessionRestore` failed with `NativeSessionResume = false`.
- Lifecycle: `TestInitializeSession_CustomACPAgentRestoresStoredSession` and
  `TestInitializeSession_CustomACPAgentRestoreFailures` recorded
  `[agent.initialize agent.session.new]` and no `agent.session.load`; after the change the stored ID
  is loaded, the capability mismatch creates one replacement, and the unrecognized failure creates
  none.
- E2E: the provider session ID changed across the restart (`mock-session-35572-1` became
  `mock-session-50888-1`); after the change the spec passes (1 passed).
- `make -C apps/backend lint`: 0 issues. `eslint --max-warnings 0` and `lint:e2e-sleeps` are clean
  on the new spec.

The E2E ran on Windows with the direct Playwright command above. Global setup there also needs
extension-less copies of `bin/kandev` and `bin/mock-agent`; that is a runner limitation, not part of
this change.

The paired requirement and system design stay `draft`: they also own REQ-AGENTS-CUSTOM-ACP-001,
whose Task 01 is still in progress.
