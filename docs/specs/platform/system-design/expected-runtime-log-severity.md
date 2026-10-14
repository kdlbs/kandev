---
status: current
system: platform
requirements:
  - REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001
created: 2026-08-28
updated: 2026-10-13
owners:
  - kandev
---
# Expected runtime log severity System Design

## Purpose and boundaries

This design preserves the severity of recognized child logs. It also assigns
non-error levels to expected task-environment and backend-restart conditions.

The design does not change process control, workspace selection, restart
detection, or public missing-session responses. Unknown child stderr remains visible at warning
level.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001` | [Child log forwarding](#child-log-forwarding), [Task-environment fallback](#task-environment-fallback), [Frontend restart recovery](#frontend-restart-recovery), [MCP observations during shutdown](#mcp-observations-during-shutdown), and [Session recovery projection](#session-recovery-projection) |

## Child log forwarding

`internal/agent/runtime/agentctl/launcher` owns the parent-side log forwarder.
The forwarder accepts these anchored child record formats:

- Zap console records with a tab-separated level field.
- Go `slog` `TextHandler` records with `time`, `level`, and a message field.
- Go `slog.Default` records with the standard log date/time prefix, level, and
  message.

The parser returns only recognized levels. The forwarder maps `DEBUG`, `INFO`,
`WARN`, and error-class levels to the equivalent parent logger method.

The parser does not infer a level from an arbitrary word. If a line does not
match a trusted format, the forwarder records it at warning level. This fallback
keeps panics, tracebacks, and malformed child output visible.

## Task-environment fallback

`internal/task/service.GetWorkspaceInfoForSession` first reads the environment
that the session references. A stale reference can return the typed
task-environment not-found error.

The service classifies that typed error before it records a log entry. It uses
the existing task-based fallback and records only debug evidence. A different
lookup error keeps the current warning and fallback behavior.

The fallback result, workspace metadata, and caller response do not change.

## Frontend restart recovery

An open application page compares its boot ID with the boot ID of a reconnected
backend. A mismatch is an expected process-lifecycle condition.

The frontend sends this condition with the allow-listed `backend-reload`
report source. The title contains `boot_id_changed` or
`settings_interlock_rejected`. The report does not contain an error-toast stack
or error object.

The backend accepts only these two titles for the `backend-reload` source. It
records the report at info level with the fixed message
`frontend backend reload required`.

The `sonner` and `toast-provider` sources keep the fixed
`frontend error toast` message at error level. A client cannot select an
arbitrary severity or log message.

## MCP observations during shutdown

`internal/agentctl/server/process.Manager` owns the durable writer and both
`PublishMCPAttachment` and `PublishMCPAttachmentAttempt`. The in-session MCP
server's attachment reporter is wired to `PublishMCPAttachment` in
`cmd/agentctl/main.go`; stateful connection teardown can report
`MCPAttachmentEvidenceConnectionClosed` after process shutdown starts.

`closeAdapterAndStdin` marks shutdown intent and closes the adapter.
`forwardUpdates` drains available terminal updates, then closes the writer
through `closeDeliveryEventWriter`. Preserve this ordering introduced by
#4432. A later MCP connection callback can still arrive after writer closure.
Writer cancellation and permanent admission closure retain their existing
bounded shutdown behavior under journal pressure.

Classify the failed MCP commit at its producer boundary. Require `StatusStopping`
or `StatusStopped` before considering any expected rejection. The private
`errDeliveryWriterClosed` sentinel identifies a writer that cannot accept the
observation. For permanent instance teardown, `CloseAdmission` cancels the
manager's lifetime context before adapter closure; an exact `context.Canceled`
write failure is expected only when that lifetime context is canceled.

After permanent journal closure, an exact `journal.ErrJournalClosed` rejection
is expected only with that canceled lifetime, a nil manager journal, the
manager's own closed-journal marker, and no journal-close error. Wrapped or
combined cancellation/journal-close errors retain ERROR, preserving independent
storage failures. A stopped status alone or a cancellation from another context
is insufficient. Active processes retain ERROR in every failure case.

The status and lifetime context are safe reads. Journal ownership evidence uses
`deliveryJournalMu`; the classifier must not acquire `Manager.mu`, which
shutdown holds while closing the adapter.

Both MCP publisher methods use one narrow classifier. Expected rejections log
at debug level and return through the existing failed-commit path. Other errors
keep the existing error message and `zap.Error` field. Do not apply this
classification to agent messages, terminal events, or general delivery errors.
Do not reopen the writer, persist directly around it, publish an uncommitted
event, or change journal close timing to make a diagnostic write succeed.

The launcher retains recognized child severity. Its JSON envelope filtering
continues to omit arbitrary child fields, including raw errors. Do not expand
that allowlist for this repair.

This classification implements
`AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.19` through `.21` and preserves
the existing [durable delivery design](durable-agent-delivery.md).

## Session recovery projection

Implements acceptance criteria `.22` through `.24`.

`internal/task/dto.EnrichSessionRecoveryBlocks` is the shared serialization
boundary for HTTP, WebSocket, boot, and orchestrator session projections.
It initializes an empty collection before querying the current persisted
generation for the session's queue incarnation, falling back to session ID
only when the incarnation is absent.

`internal/task/repository/sqlite.GetCurrentHarnessSessionGeneration` returns
`models.ErrTaskSessionNotFound` when its generation query has no rows. A task
session can exist before native initialization commits its first generation.
The projection shall recognize that typed no-generation result and return
the empty collection without a warning. Keep the repository's strict result
unchanged because recovery and admission consumers rely on it.

Limit this handling to the generation lookup. Do not suppress an error from
`ListOpenSessionRecoveryBlocks`, including a not-found sentinel there, or an
unrelated generation-read error. Existing caller warning paths remain effective.
No state is created or repaired during serialization. Public session lookup and
authorization still occur before projection and retain their missing-session
response. Preserve current-generation filtering after successful initialization.

The frontend's `session-state-reconciler.ts` performs bounded conditional HTTP
reads every 750 ms for busy sessions. DTO enrichment runs before ETag response
selection, so an unchanged response can still trigger the defective warning.
This repair does not change polling, ETags, or startup scheduling.

Integration tests must use a real repository and persisted task session with
no generation; a nil-success fake alone does not represent this repository
contract. Pair the regression with unexpected-error and generation/block
filtering tests. Check HTTP conditional responses and orchestrator events.

See [durable harness boundaries](../../../decisions/2026-09-10-durable-harness-session-boundaries.md)
for the existing persistence and recovery-admission guarantees.

## Failure and recovery

An unknown child record remains a warning. This rule prevents a parser change
from hiding unstructured failures.

An unexpected task-environment lookup error remains a warning. The task-based
fallback can still return usable workspace information, but the storage error
remains diagnostic evidence.

An invalid frontend report remains a bad request. A valid reload report cannot
reduce the severity of an actual toast report because each source has a fixed
severity.

## Observability

The parent log keeps the original child line and `stream` field. Only the parent
severity changes when the parser recognizes the child severity.

The stale task-environment path keeps the session and environment identifiers
at debug level. Unexpected lookup errors keep the existing warning fields.

The reload entry keeps the browser context and stable recovery signal. It does
not use the error-toast message or a synthetic error stack.

## Test strategy

Table tests cover Zap, Go `slog`, malformed, and unstructured child lines.
Observer-backed service tests cover typed not-found and unexpected lookup errors.

Observer-backed frontend-report tests cover the reload info entry and the
unchanged error-toast entry. Frontend tests cover the distinct report source
and the absence of a synthetic toast stack.

Observer-backed process tests reproduce a committed MCP observation followed
by real writer closure and a late connection-close observation. Cover both
MCP publisher entry points, active-process and unrelated-error controls,
legacy delivery without a journal, and publication after a supported restart.
Retain the full-journal shutdown regression so producer cancellation continues
to release teardown without waiting for ACK.

## Implementation records

- [MCP attachment shutdown log classification](../../../plans/mcp-attachment-shutdown-logging/plan.md).
- [Fresh-session recovery projection](../../../plans/fresh-session-recovery-projection/plan.md).
