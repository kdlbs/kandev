---
id: "04-kanban-reliability-verification"
title: "Verify Kanban completion reliability"
status: done
wave: 3
depends_on: ["01-task-only-eager-tool", "02-canonical-tool-naming", "03-pin-and-reconnect-claude-acp"]
plan: "plan.md"
spec: "../../specs/workflow/explicit-completion-signal/spec.md"
---

# Task 04: Verify Kanban completion reliability

## Acceptance

- Every spec scenario has automated evidence or a documented manual-only reason.
- Task mode remains callable before and after reconnect-shaped load; Office mode still excludes the tool in registration and prompt text.
- Verification reports bridge version, transport used, and remaining uncertainty about the original field disconnect.

## Verification

```bash
cd apps/backend && rtk go test ./internal/mcp/server ./internal/sysprompt ./internal/agent/agents ./internal/agentctl/server/api ./internal/agentctl/server/adapter/transport/acp -count=1
```

Optional trusted-machine probe:

```bash
cd apps/backend && KANDEV_DEBUG_AGENT_MESSAGES=true KANDEV_MCP_LOG_FILE=/tmp/kandev-mcp.log go run ./cmd/acpdbg probe claude-acp
```

## Files Likely Touched

- Tests in the packages named above only when a scenario lacks coverage.
- No production files unless a new red reproduction justifies a separate fix packet.

## Inputs

- Completed Tasks 01-03.
- `/acp-debug`, `/qa`, and `/verify` skill procedures.

## Output Contract

Act as QA. Report scenario-by-scenario evidence, exact commands/results, logs captured, cleanup, residual risks, blockers, and task status. Do not edit `plan.md` or broaden the fix.

## QA Evidence

### Scenario matrix

| Spec scenario | Evidence | Result |
|---|---|---|
| Task-mode tool list contains discoverable `step_complete_kandev` without eager-load metadata | `TestServerStepCompleteTool_TaskOnlyAndDiscoverable` serializes the tool schema and rejects `_meta`, while prompt coverage directs deferred clients to tool search/discovery. | Pass |
| Office/config/external exclude the completion tool, and Office first-turn context excludes canonical and qualified forms | The task-only mode table checks all three restricted modes. `TestSyspromptToolNames_ExactlyMatchMCPOfficeMode` requires the Office prompt to match the registered Office inventory exactly. Mode-aware injector/orchestrator tests replace stale task context with Office context and stale Office context with signal-gated task context. | Pass |
| A loaded task session can list and call the completion tool without another user message | `TestMCPToolCatalogRemainsAvailableAfterAgentSessionLoad` performs MCP initialize, `tools/list`, and `tools/call`, runs `agent.session.load`, then repeats initialize/list/call successfully. API capture tests also prove both `session/new` and `session/load` receive local HTTP and SSE entries. | Pass for the session-load-shaped continuity contract |
| A signal-gated step does not advance on a bare halt, but advances with a matching signal | `TestProcessOnTurnComplete_ExplicitSignalGating` covers absent, matching, stale, and legacy signals; `TestOnStepCompletionSignaled` covers the post-halt subscriber and clarification barrier. | Pass for backend gating |
| The halt-without-signal manual completion fallback is shown | Backend tests prove the task correctly stays on its current step, but no shipped task-chat fallback UI or manual-signal write path was found. `StepCompletionSourceManualFallback` has only model/comment and metadata decoder references outside tests. Recovery today is retry/reconnect or a normal human workflow move. | Verified pre-existing/out-of-scope product gap; not passed |
| A client can associate canonical and qualified names | Prompt tests require canonical-name guidance and the example `mcp__kandev__step_complete_kandev` only on signal-gated task steps. Prompt-sync extraction normalizes the qualified example to the canonical registered name. Public-doc validation passed. | Pass |
| Claude ACP command paths consistently select the bridge package | `TestClaudeACPCommandsUseUnversionedBridgePackage` verifies normal, runtime, inference, and install paths use unversioned `@agentclientprotocol/claude-agent-acp`. The opt-in real wakeup E2E fixtures retain their pre-existing `0.31.1` version. Existing ACP initialization records the bridge-reported `agent_name` and `agent_version` in tracing/logging and returns them from agent initialization. | Pass for command consistency; live self-report not probed |

### Commands and results

- `cd apps/backend && GOCACHE=/tmp/kandev-go-cache rtk go test ./internal/agent/agents ./internal/agentctl/server/api ./internal/agentctl/server/adapter/transport/acp -run 'Test.*(Claude|Mcp|MCP|LoadSession|NewSession)' -count=1` passed 45 tests in 3 packages.
- `cd apps/backend && GOCACHE=/tmp/kandev-go-cache rtk go test ./internal/orchestrator -run 'Test(ProcessOnTurnComplete_ExplicitSignalGating|ProcessOnTurnCompleteViaEngine_BlocksWhileClarificationPendingEvenWithSignal|OnStepCompletionSignaled|StartCreatedSession_ReplacesOfficeContextForSignalGatedTask)' -count=1` passed 15 tests.
- `cd apps/backend && GOCACHE=/tmp/kandev-go-cache rtk go test ./internal/mcp/server ./internal/sysprompt -run 'Test.*(StepComplete|KandevContext|Sysprompt|Office|Context)' -count=1` passed 47 tests in 2 packages after the prompt-mode repair settled.
- `cd apps/backend && GOCACHE=/tmp/kandev-go-cache rtk go test ./internal/orchestrator -run 'Test(StartCreatedSession_(WrapsFirstPromptWithKandevSystemBlock|ReplacesOfficeContextForSignalGatedTask|OfficeTask)|ProcessOnTurnComplete_ExplicitSignalGating|OnStepCompletionSignaled)' -count=1` passed 16 tests.
- Repeated focused stability runs passed: bridge command consistency 2 tests, API new/load/catalog continuity 6 tests, transport negotiation 6 tests, and final ModeTask/Office prompt assertions 20 tests.
- `node --test scripts/validate-public-docs.test.mjs` passed 1 test; `node scripts/validate-public-docs.mjs` validated 40 pages.
- `git diff --check` passed.

The exact five-package command ran once and reported 790 passes, 2 failures, and 5 skips. Both failures were sandbox-only loopback panics from `httptest` (`TestAskUserQuestion_StreamsKeepAliveDuringWait` and `TestAgentStreamWS_RequestResponseFlow`: `listen tcp6 [::1]:0: socket: operation not permitted`). A skip-based retry found the next two loopback-dependent tests and was not repeated. Focused tests exercising every changed completion path use `httptest.NewRecorder` or in-memory ACP pipes and passed; the blocked socket tests are unrelated to this change.

All `rtk` runs also printed a non-fatal read-only warning for `/run/user/0/fnm_multishells`; the Go test summaries above are the command results.

### Transport and probe limits

- New/load API injection supplies Streamable HTTP first (`/mcp`) and SSE fallback (`/sse`). ACP negotiation tests prove HTTP is selected when both capabilities are supported and SSE is selected for an SSE-only bridge.
- Production Claude bridge commands use unversioned `@agentclientprotocol/claude-agent-acp`. The existing ACP initialize path logs and traces the bridge-reported version.
- No live `acpdbg` probe was run: this is a restricted sandbox with loopback/network limitations, and `apps/backend/bin/acpdbg` is not built. Running the `npx` bridge would also exercise external credentials/network rather than the deterministic repository contract.
- The original field disconnect/reconnect trigger and deferred-catalog loss remain unproven because the reported session had no ACP/MCP/stdout trace. The reconnect-shaped test proves Kandev reconstructs and serves the catalog after load; it does not simulate an upstream Claude MCP transport disconnect.

### Verdict

Task-only discovery without eager metadata, Office exclusion, canonical naming, unversioned bridge command consistency, HTTP/SSE new/load wiring, and load-shaped list/call continuity are verified. The reliability fix is ready with the stated external-disconnect uncertainty. The spec's pre-existing manual-fallback UI claim is not shipped: backend gating leaves the task on its current step, while actual recovery is retry/reconnect or a normal human workflow move. Handle the missing fallback as a separate bounded remediation or correct the spec.
