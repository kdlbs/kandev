# Agent Orchestrator comparison

Inspected on 2026-09-24 at commit `1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5`.
The project has an [Apache-2.0 license](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/LICENSE).
An explicitly requested explorer inspected the external project. The primary session compared the current Kandev implementation and design.
These are recommendations for the existing work orders. This investigation does not change their completion status.

## Recommended improvements

### 1. Keep approval waits outside notification dispatch

The external RPC reader starts server-request handlers asynchronously. Notifications and RPC responses can continue while a user considers an approval.
See [rpc.go](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/codexappserver/rpc.go#L196).

Kandev's current `pkg/codexappserver/client.go` dispatches requests and notifications in one synchronous loop.
The adapter's permission handler can wait for user input. During that wait, later notifications cannot reach the adapter.
This includes child activity and resolved-request notifications. The bounded queue can eventually fill and close the connection.

For work orders 01 and 02, separate request completion from ordered notification dispatch.
Bound outstanding requests, cancel their contexts on disconnect, and preserve request ownership.
Do not copy unrestricted goroutine creation.
Test an unanswered approval while child output, a normal RPC response, cancellation, and request resolution arrive.

The external notification relay has a 32 MiB budget. Once full, it blocks the reader.
Its normalized event queue can also drop events after five seconds.
Do not adopt those behaviors as evidence of end-to-end delivery guarantees.
See [relay](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/codexappserver/rpc.go#L106)
and [event delivery](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/codexappserver/conversation.go#L260).

### 2. Preserve the provider's approval choices

The external adapter retains raw `availableDecisions`, validates the selected choice, and returns the original structured decision.
It rejects stale requests and choices that were not offered.
See [approval handling](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/codexappserver/conversation.go#L639)
and [tests](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/codexappserver/driver_test.go#L503).

Kandev's current adapter offers three fixed choices and handles only command and file approvals.
Its design already requires questions and `serverRequest/resolved` handling.
For work order 02, keep provider payloads in the backend and expose normalized option IDs to the UI.
Validate both request ownership and membership in the offered choices before replying.
Add fixtures for structured policy amendments, stale selections, resolution before selection, and unsupported mandatory requests.

Do not copy the external question path unchanged. It forwards raw answers without validating question IDs or answer shapes.
See [question replies](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/codexappserver/conversation.go#L914).

### 3. Distinguish live reconnect from saved-thread resume

The external project has a persistent process host. Closing the controller detaches it; termination destroys the host.
Reattaching to the same initialized process skips initialization and thread resume.
A new process resumes the saved thread and does not silently create another one.
See [driver lifecycle](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/codexappserver/driver.go#L354).

Its host preserves request-ID continuity and replays unanswered server requests.
See [host correlation](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/persistenthost/host.go#L793)
and [replay tests](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/persistenthost/host_test.go#L421).

For work orders 02 and 03, test these lifecycle distinctions through Kandev's existing agentctl ownership.
Keep the current process manager boundary. A second persistent host is not recommended.
If a replacement RPC client can attach to a surviving native stream, it must not reuse IDs while old responses remain possible.
An application reconnect that retains the same RPC client does not need a second native initialization.

### 4. Test coverage against the pinned protocol

The external conformance suite compares its method inventory with generated methods and enumerates approval methods.
This detects missing handlers when the generated schema changes.
See [conformance tests](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/codexappserver/conformance_test.go#L128).

For work orders 01 and 07, extend Kandev's schema digest check with explicit method coverage and payload fixtures.
Classify each required server request as supported or deliberately rejected.
Report unavailable installed-binary validation separately from a passing compatibility check.
Method names alone do not prove payload compatibility.
The external generated schema targets 0.146.0; retain Kandev's reviewed 0.154.0 baseline.

For `codexdbg`, use these scenarios as capture and offline-inspection fixtures.
Include unresolved requests, child events during approval, replayed usage, and model changes.
The existing shared-client/debugger design remains appropriate.

## Usage and costs

The external native adapter projects cumulative token totals and uses `last.totalTokens` for context occupancy.
It does not establish exact per-response dollar charges.
See [usage normalization](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/codexappserver/normalize.go#L559).

Its separate rollout observer updates provider and model attribution from `session_meta` and `turn_context` records.
Changes apply to subsequent usage. This is useful evidence for model-switch and provider-reroute tests.
See [temporal attribution](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/observe/usage/parser.go#L389).
Kandev should preserve the model effective for each measured response or turn, including delayed observations.
The currently selected model alone is insufficient when the provider supplies more precise attribution.

The observer also handles counter resets, repeated totals, cached-input subsets, and reasoning-output subsets.
See [counter handling](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/observe/usage/parser.go#L627).
These cases reinforce work order 04. They do not justify a second usage writer or a rollout-file dependency in Kandev.
Keep response-level observations, labeled fallback measurements, and calculated-cost provenance in the existing ledger.
The built-in usage display remains independent of the session-cost plugin.

## Subagents and forks

The external native normalizer does not map `collabAgentToolCall`, despite its presence in the generated schema.
Several normalized events also omit provider thread identity.
See [item normalization](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/codexappserver/normalize.go#L958).
Do not use this implementation as the complete reference for child visibility or attribution.
Keep Kandev's explicit thread/turn/item identities and independent child lifecycle.

The external project scopes projected IDs while retaining native IDs for provider calls.
This reinforces collision tests across resumed and forked Kandev sessions.
See [identity scoping](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/codexappserver/provider_scope.go#L10).

Its fork path passes `lastTurnId`, inherits the source cwd, and requires a returned thread ID.
Resume reapplies configuration instead of assuming history retains every setting.
See [history operations](https://github.com/Untrivial-ai/agent-orchestrator/blob/1fcf35bb720272fd0a9a34d4d4ebd092e2c506f5/backend/internal/adapters/chatdriver/codexappserver/history.go#L367).
These choices support work order 06's explicit boundary and shared-workspace behavior.

## Validation limits

The explorer ran `go test -race rpc.go rpc_test.go` in the external package. It passed using in-memory pipes.
Other cited tests were inspected, not executed. No real agent or authenticated model turn ran.
No Kandev production files were changed by this investigation.
Current implementation gaps above describe the inspected working tree, which remains in progress.
