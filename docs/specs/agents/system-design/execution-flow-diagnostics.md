---
status: current
system: agents
updated: 2026-11-07
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
---

# Execution flow diagnostics

The existing structured log sinks carry an `execution flow` envelope at actual
boundaries. Request acceptance, asynchronous dispatch, executor preparation,
process start, session restore, turn persistence, first message segment, tool
start, durable delivery receipt, terminal settlement, and reservation release
are distinct outcomes. A received request is not evidence of a model call or
useful work. A completed turn is not evidence that its task is complete.

The envelope joins request/correlation, task, session, attempt, turn, execution,
message, submission, stream/sequence, profile, workflow/step/entry and reservation
identities when those identities exist at that boundary. Route, harness and
prompt generations have separate fields. The backend boot ID is the same one
reported by system info; agentctl events join to that boot through execution ID.
Paired operations record elapsed milliseconds. Admission records known/unknown
population, overall and grade limits, and the concrete waiting reason.

Success transitions are not sampled. A first message segment uses the existing
append marker, not a new unbounded deduplication map. Token/reasoning appends and
status polls do not produce flow events. Capacity wait logging occurs when its
reason changes. Existing bounded asynchronous sinks and rotation remain in use;
sink loss counters remain authoritative if storage cannot keep up.

No prompt, response, command, path, environment, arbitrary metadata or raw error
is part of the envelope. Identifier fields are bounded to 192 ASCII identifier
characters. The agentctl relay accepts only the typed envelope on the fixed flow
message, caps its encoded input at 8 KiB, and discards unknown properties. Other child log fields continue through the existing validated relay. Startup refusal names the owner-claim,
identity, revision, storage or activation-persistence boundary rather than using
`generic_launch_failure` as a root cause.

These logs prove native receipt and execution boundaries. They do not infer the
semantic relevance of an agent's output or a separate desktop conversation's
queue behavior; those require the corresponding official conversation records.
Operational acceptance still requires a real same-session first tool, useful
output, completion, settled/released capacity and a subsequent assignment after
installation. Isolated fixtures do not establish installed recovery or sustained
operation near the configured overall and grade limits.
