---
status: current
system: platform
requirements:
  - REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001
created: 2026-08-28
updated: 2026-09-05
owners:
  - kandev
---
# Expected runtime log severity System Design

## Purpose and boundaries

This design preserves the severity of recognized child logs. It also assigns
non-error levels to expected task-environment and backend-restart conditions.

The design does not change process control, workspace selection, restart
detection, or returned errors. Unknown child stderr remains visible at warning
level.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001` | [Child log forwarding](#child-log-forwarding), [Task-environment fallback](#task-environment-fallback), and [Frontend restart recovery](#frontend-restart-recovery) |

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
