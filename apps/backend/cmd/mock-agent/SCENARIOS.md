## Mock-agent scenarios

Use `/e2e:<name>` for a registered scenario. `scenarioRegistry` in `scenarios.go` owns the names.
Add a registry entry and a scenario function that uses `emitter.go`.
Use `e2e:<directive>(...)` for an inline script. `script.go` and `script_test.go` define its syntax.
Keep scenario delays short. The aliases and their arguments are in `handler.go`.

`/bulk[:N]` emits separate message rows for pagination tests. `/slow [duration]` accepts Go duration units;
positive bare numbers mean seconds. Use a short explicit duration for handler tests.
The background and async-subagent aliases exercise foreground-yield and detached-work events.

## Errors and recovery fixtures

A registered scenario emits updates; it cannot fail the prompt. To return a real JSON-RPC error,
handle the prompt and return an `acp.RequestError`. `/overloaded[:N]` is the example.
Its counter uses a session-keyed temporary file so it survives process restart.

Capacity scenarios in `capacity_continuity.go` model a failed turn with a retained ACP runtime.
The OpenCode variants in `opencode_capacity.go` use the real provider dialect and error shape.
They exercise retry, cancellation, completed effects, and unresolved tools. Continuation does not replay tools.
See those files for the scenario names and defaults. `CloseSession` clears their temporary state.

`/transport-lost[:N]` returns a remote error on a live connection; it does not simulate transport death.
Use `/crash` or close the pipe or subprocess for actual transport loss.
The `/prompt-error*` fixtures return generic retained-turn errors. Their optional `:N` suffix controls failures.

Set `E2E_MOCK_AGENT_ACP_TRACE_FILE` to record opt-in ACP traces. They include process, connection,
and session IDs and request types. Prompt blocks record lengths and digests, not raw image or audio bytes.
Set `E2E_MOCK_AGENT_FAIL_INITIALIZE=true` to fail before a native session exists; remove it before recovery.

## Emitter helpers

Use `emitter.go` for text, thoughts, plans, tool calls, permissions, monitor events, and subagent events.
The `git-commit-permission` scenario creates a commit only after permission is granted.
For Claude `_meta` tool calls, use `startMonitorTool` or `startSubagentTool` as the pattern.
They set `tc.Meta` through a local option closure.
