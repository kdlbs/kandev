# mock-agent — scenario library for reproducing agent output locally

Scoped guidance for `apps/backend/cmd/mock-agent/`. The mock agent satisfies the same ACP surface a real agent does, so the rest of the stack (orchestrator, lifecycle manager, agentctl, web UI) sees it as an ordinary agent. It runs in dev/e2e mode behind `KANDEV_MOCK_AGENT=true` (set automatically by the `dev` and `e2e` profiles — see `profiles.yaml`).

## Two ways to drive it

**1. Predefined scenarios — `/e2e:<name>`**

`handler.go` routes any prompt starting with `/e2e:` to `emitPredefinedScenario(e, name)`, which looks `name` up in `scenarioRegistry` (`scenarios.go`). The registry is the single source of truth for scenario names — to add a scenario, add one map entry and one `scenario<Name>(e *emitter)` function.

Friendly aliases live in `handler.go` next to the dispatcher: `/ask-single`, `/ask-multiple`, `/crash`, `/todo`, `/mermaid`, `/markdown`, `/sleep [n]`, `/tool:<name>`, `/subagent`, `/subtask`.

**2. Inline scripts — `e2e:<directive>(...)`**

`script.go` parses prompts starting with `e2e:` (no leading slash) as multi-line scripts. Supported directives include `e2e:message(...)`, `e2e:thinking(...)`, `e2e:tool_use("Name", {...})`, `e2e:tool_result(...)`, `e2e:delay(ms)`, the Monitor-flavoured `e2e:monitor_start/event/end`, and the MCP-flavoured `e2e:mcp:kandev:<tool>({...})`. Comments start with `#`, blank lines are ignored. Tests live in `script_test.go`.

Use predefined scenarios for anything you want to trigger from the slash menu or replay across sessions; use inline scripts for ad-hoc shapes inside a single E2E spec or manual session.

## Adding a scenario to reproduce a UI rendering bug

When a real-agent message renders poorly (markdown table, tool card, diff panel, plan summary, etc.), the cheapest reliable repro is a mock scenario that emits a representative payload. Pattern:

1. Add `"<your-name>": scenario<YourName>` to `scenarioRegistry` in `scenarios.go`.
2. Implement `scenario<YourName>(e *emitter)` using the helpers in `emitter.go` — `e.text`, `e.thought`, `e.startTool` / `e.completeTool`, `e.plan`, etc. Keep delays modest (`fixedDelay(100)`) so manual repro feels live but tests stay fast.
3. Trigger from any task in dev/mock mode by typing `/e2e:<your-name>` as the prompt.

This avoids burning real-agent tokens, gives you a deterministic payload to iterate against, and the scenario stays around as a permanent regression hook — anyone can re-trigger it after the fix.

## Special case: emitting a real prompt-time ACP *error*

Scenarios in `scenarioRegistry` can only emit `SessionUpdate` notifications — they cannot make the prompt itself fail. When you need a real JSON-RPC error response (the kind the backend turns into `agent.failed` with a populated `data.ErrorMessage`), intercept the command in the `Prompt` method (`main.go`) and return a non-nil `error` — typically `&acp.RequestError{Code, Message, Data}`, whose `Error()` serializes the exact `{"code":...,"message":...,"data":...}` envelope a real agent sends.

`/overloaded[:N]` is the reference example (`handler.go: handleOverloaded`): it returns the production `529 Overloaded` error for the first `N` prompts of a session (default 1), then recovers with a normal text response. The orchestrator's backoff retry tears the agent process down and relaunches it between attempts, so the fail-count is persisted in a **temp file** keyed by the session id (`overloadedCounterPath`) rather than an in-memory map — it survives the relaunch (and is cleaned up on recovery and in `CloseSession`). Use `/overloaded` to demo the yellow retry status, or a large `N` like `/overloaded:9` to keep failing so the retry loop stays visible / exhausts to the red recovery banner.

## Emitter helpers worth knowing

`emitter.go` wraps the ACP `sessionUpdater` with helpers that shield scenarios from SDK plumbing:

- `e.text(s)` / `e.thought(s)` — plain agent message vs reasoning channel.
- `e.startTool(id, title, kind, input, locs...)` / `e.completeTool(id, output)` — paired tool-call lifecycle.
- `e.plan(entries)` — ACP plan updates.
- `e.startMonitorTool(id, taskID, command)` / `e.emitMonitorEvent(taskID, body)` / `e.endMonitorTool(id)` — reproduces the two-frame Monitor wire pattern the kandev ACP adapter recognises.
- `e.startSubagentTool(...)` / `e.completeSubagentTool(...)` — claude-style subagent (Task) frames with the `_meta.claudeCode` Agent marker and result metrics.
- `e.requestPermission(...)` — interactive permission flow for scenarios that need an Allow/Reject decision.

### Emitting `_meta`-tagged tool calls

The acp-go-sdk has no `WithStartMeta`/`WithUpdateMeta`. To reproduce claude-agent-acp's `_meta.claudeCode.*` wire shape (`Monitor`, `Agent`-subagent), set `tc.Meta` via a local option closure — see `startMonitorTool` / `startSubagentTool` in `emitter.go`. Use these instead of building raw `SessionNotification`s when you need to exercise the adapter's `_meta` recognisers.

## Rebuild before running e2e (easy to miss)

The web e2e suite runs the **prebuilt host binary** `apps/backend/bin/mock-agent` (resolved via `PATH`); the `containers` project additionally uses `bin/mock-agent-linux-amd64`. `global-setup.ts` only checks the binary **exists**, not that it's current — so a stale binary silently runs the OLD behavior and your new scenario/edit won't take effect. After changing anything under `cmd/mock-agent`:

```bash
make -C apps/backend build-mock-agent          # host binary (default suite)
make -C apps/backend build-mock-agent-linux    # only for the `containers` project
```

## Tests

`mock_agent_test.go` exercises the top-level handler dispatch. `script_test.go` covers the inline-script parser end-to-end. New scenarios don't typically need a unit test — the assertion is that the scenario name is registered in `scenarioRegistry` and the payload looks right when triggered manually. If a scenario contains conditional logic worth pinning, add a focused test that constructs an emitter with a fake `sessionUpdater` and asserts on the emitted notifications.
