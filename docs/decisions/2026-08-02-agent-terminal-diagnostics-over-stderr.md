# ADR-2026-08-02-agent-terminal-diagnostics-over-stderr: Capture Agent Terminal Diagnostics From Managed Stderr

**Status:** accepted
**Date:** 2026-08-02
**Area:** backend, frontend, protocol, security

## Context

OpenCode can log a terminal provider stream failure while leaving its ACP
`session/prompt` request open. Kandev then sees an indefinitely quiet running
turn even though the agent knows it cannot continue. The same diagnostic is
available in OpenCode's private data-directory log and, when explicitly
enabled, on the stderr of the `opencode acp` process that agentctl owns.

## Decision

Agent-private files are not a Kandev runtime contract. When an agent supports
explicit diagnostic emission, Kandev enables error-only output on the managed
agent subprocess and consumes it inside agentctl, in the executor where the
process runs.

An out-of-band diagnostic may settle a prompt only when an agent-specific
parser validates the terminal signature and correlates it to the current agent
type, active provider session, and active foreground prompt. Parsers retain
only allowlisted fields, sanitize before crossing the agentctl boundary, and
never forward raw log lines. Unmatched, malformed, background, stale, or
unsupported diagnostics are ignored.

For OpenCode, a correlated `stream error` is terminal evidence, not an
inactivity inference. It cancels the held prompt operation and enters Kandev's
existing generation-scoped error lifecycle. Silence without such evidence
continues to follow
[ADR-2026-07-29-agent-stall-user-controlled-recovery](2026-07-29-agent-stall-user-controlled-recovery.md).

Raw stderr is inspected only in memory by the protocol-specific parser. For
OpenCode, the process manager applies the parser's safe message projection
before writing generic logs, the recent-stderr ring, or process-exit events;
malformed and unrelated records are excluded. Other agents retain their
existing stderr behavior. Raw OpenCode lines are never persisted, included in
browser payloads, or exposed through a general API for reading agent logs. The
observable behavior is specified in
[`docs/specs/agent-stall-recovery/spec.md`](../specs/agent-stall-recovery/spec.md).

## Consequences

Provider failures that an ACP bridge swallows can become immediate, truthful
session failures across local, container, and remote executors without granting
the central backend filesystem access. Existing lifecycle reconciliation,
recovery messages, and provider-error classification remain the single path
after agentctl validates the diagnostic.

Kandev takes on a small agent-compatibility parser for each supported stderr
format. A vendor format change degrades safely to ACP plus advisory stall
recovery rather than guessing. Enabling unsupported CLI diagnostic flags can
affect startup, so the managed runtime contract and compatibility tests must
cover those arguments.

## Alternatives Considered

- **Tail `~/.local/share/opencode/log/opencode.log`.** Rejected because it is a
  private, rotating, multi-session file containing unrelated paths, URLs, and
  identifiers, and the central backend cannot access every executor's home.
- **Expose the recent stderr buffer to the browser.** Rejected because raw
  stderr is broader than the user-facing failure and may contain sensitive or
  unrelated diagnostics.
- **Treat any OpenCode stderr error as terminal.** Rejected because title,
  background, stale, and concurrent-session work can emit unrelated failures.
- **Rely only on the inactivity watchdog.** Rejected because it delays a known
  terminal failure and continues to describe a failed turn as running.
- **Add a universal hard prompt timeout.** Rejected because quiet long-running
  work is legitimate and silence is not terminal evidence.
- **Wait only for an upstream OpenCode ACP fix.** Rejected as the sole remedy;
  upstream conformance remains desirable, but Kandev can safely use diagnostics
  from the subprocess it already supervises.
