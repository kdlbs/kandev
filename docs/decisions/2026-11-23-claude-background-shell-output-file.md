# ADR-2026-11-23-claude-background-shell-output-file: Tail the Claude background shell output file named by its launch result

**Status:** accepted
**Date:** 2026-11-23
**Area:** backend

## Context

Claude detached shells show "Output is not available for this job" in the
background jobs viewer (GitHub issue #5439). Claude Code writes each backgrounded
Bash command's output to `<temp>/claude-<uid>/<project>/<native-session-id>/tasks/<id>.output`
and names that file in the Bash launch result. The ACP bridge streams the file
only to ACP v2 clients and to AIR clients that advertise `asyncTasks`. Kandev is
neither, and the
[async-task proposal](2026-11-01-claude-async-task-lifecycle-capability.md) was
rejected because the pinned SDK cannot decode those frames and the capability
adds a follow-up turn.

## Decision

agentctl extracts the output path from the launch result of a tracked, verified
Claude detached shell and tails that file into the existing
`background_work_output` stream. The path is readable only when it is an
absolute `tasks/<id>.output` regular file whose session directory equals the
adapter's current native session ID. No other provider-reported path outside the
workspace becomes readable through this mechanism.

## Consequences

- Claude detached shells advertise `Output: "stream"` and show live, bounded
  output on desktop and phone with no web change.
- Extraction depends on Claude Code's "Output is being written to:" wording.
  The bridge's own `asyncTaskOutputFilePath` depends on the same wording. If it
  changes, output falls back to the explicit unavailable state; settlement is
  unaffected.
- Reading runs only for verified bridge versions, whose tool-scoped terminal
  exit stops it. One poller per adapter follows at most 20 jobs and drops a
  job after 30 minutes without new output, so a missed exit cannot cause
  unbounded polling.

## Alternatives considered

- **Advertise AIR `asyncTasks`.** Rejected by the earlier ADR; still blocked by
  the SDK and the follow-up-turn product decision.
- **Adopt ACP v2 sessions.** The bridge streams the file to v2 clients, but the
  protocol migration is far larger than this defect.
- **Allow any provider-reported path.** Broadens the read boundary beyond the
  verified Claude task file shape.
