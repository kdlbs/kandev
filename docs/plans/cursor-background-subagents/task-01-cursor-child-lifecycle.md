---
id: "01-cursor-child-lifecycle"
title: "Handle Cursor child lifecycle on the interactive connection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-005
acceptance_criteria:
  - AC-AGENTS-SUBAGENT-LIFECYCLE-005.1
  - AC-AGENTS-SUBAGENT-LIFECYCLE-005.2
  - AC-AGENTS-SUBAGENT-LIFECYCLE-005.3
  - AC-AGENTS-SUBAGENT-LIFECYCLE-005.4
system_design:
  - ../../specs/agents/system-design/subagent-provider-lifecycle.md
---

# Handle Cursor child lifecycle on the interactive connection

1. Start with a failing pipe-based regression, TestCursorBackgroundSubagentLifecycle, beside the ACP adapter tests. Send the two captured discriminators as JSON through the production connection. Check the initialize payload, child routing, one running and one completed observation, and one parent prompt. A typed adapter test alone bypasses the SDK decoding boundary.
2. In apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_helpers.go add the Cursor interactive capability. In the connection path used by adapter.go handle the vendor shapes before generated union decoding loses their identity. Keep ordinary frames ordered and apply the same notification barrier that precedes prompt completion.
3. In dialect_cursor.go and adapter_cursor.go correlate the parent spawn, async tool result and child state. Use the existing Background Work publisher and projection. Extend the adapter and orchestrator regressions for overlapping children, terminal replay, reversed launch/spawn order, a child-session message, a replaced binding, disabled projection, and legacy Cursor metadata.
4. From apps/backend run the following focused tests. Exercise the real Kandev session with an authenticated Cursor CLI. Ask a background child to sleep, then write a marker; verify one active job while it runs, a completed History entry after reload, and one parent report without a second prompt. Check the public agent documentation for the new support.

```sh
go test -tags fts5 ./internal/agentctl/server/acp ./internal/agentctl/server/adapter/transport/acp ./internal/orchestrator -run 'Cursor|Subagent' -count=1
```

Complete the work order when the focused tests and the Kandev session check pass. Record the Cursor CLI version and results here.

## Dependency found during implementation

A Go replay of the live negotiated spawn and completion notifications through
the pinned ACP SDK (27925bf88fae) returned nil decoding errors for both frames.
Neither frame populated SessionUpdate.SubagentUpdate. The typed callback loses
the original discriminator and raw child session ID before the adapter sees it.
The existing extension notification hook operates on JSON-RPC method names;
both of these frames use the known method `session/update`.

Add an ordered raw update decoding hook in kdlbs/acp-go-sdk, with a fixture for
an unknown discriminator, then update the backend module pin. Keep recognition
of Cursor's two discriminator values in the Kandev client. This requires a
companion SDK PR. The prior [transport decision](../../decisions/2026-08-20-acp-client-non-underscore-extension-methods.md)
explains the cost of duplicating SDK framing in the application.


## Implementation results

[Kandev ACP SDK PR 9](https://github.com/kdlbs/acp-go-sdk/pull/9) merged as e577f3eb8764. The Cursor interactive connection now opts into the raw callback and sends child lifecycle updates through the adapter's existing ordered notification queue.

Cursor CLI 2026.10.19-6bbf2c4 ran twice against the patched Kandev adapter. A single prompt launched a child, which wrote CURSOR_DONE into done.txt. Both runs emitted running and completed observations. In the second run the harness also checked that the parent reported CURSOR_DONE after the terminal observation.


A disposable Kandev backend using the same CLI passed a Chromium check. It launched a real child from the initial task description. The workload API returned one running subagent and the composer displayed its chip. The child completed, the parent reported the marker, and the same job appeared as Completed in History after reload. The fixture shut down. The focused ACP and orchestrator suites, adapter race tests, changed-package Go lint, and specification and public documentation checks passed.

Review regressions cover two overlapping children that share a launch call ID in different turns, and a running child's transition to unknown across a context reset. Both tests failed before the correction. The adapter now correlates dispatch by turn and call ID, tracks completion by child session ID, and forwards the retired observation through the lifecycle reset buffer.

A further replay test reproduced a completed child claiming a reused launch after its prompt was swept. Kandev now retains the opaque IDs of projected terminal children until native session replacement. The regression verifies that duplicate frames cannot hide the next child. A table-driven test covers completed, failed, cancelled, and disconnected outcomes.
