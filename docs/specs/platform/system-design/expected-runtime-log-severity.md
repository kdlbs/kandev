---
status: current
system: platform
requirements:
  - REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001
created: 2026-08-28
owners:
  - kandev
---
# Expected runtime log severity System Design

## Purpose and boundaries

This design preserves the severity of recognized child logs. It also removes
warnings from one expected task-environment fallback.

The design does not change process control, workspace selection, or returned
errors. Unknown child stderr remains visible at warning level.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001` | [Child log forwarding](#child-log-forwarding) and [Task-environment fallback](#task-environment-fallback) |

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

## Failure and recovery

An unknown child record remains a warning. This rule prevents a parser change
from hiding unstructured failures.

An unexpected task-environment lookup error remains a warning. The task-based
fallback can still return usable workspace information, but the storage error
remains diagnostic evidence.

## Observability

The parent log keeps the original child line and `stream` field. Only the parent
severity changes when the parser recognizes the child severity.

The stale task-environment path keeps the session and environment identifiers
at debug level. Unexpected lookup errors keep the existing warning fields.

## Test strategy

Table tests cover Zap, Go `slog`, malformed, and unstructured child lines.
Observer-backed service tests cover typed not-found and unexpected lookup errors.
