---
status: current
system: agents
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-004
  - REQ-AGENTS-BACKGROUND-WORK-010
---

# Claude background shell output

## Ownership and scope

Agents owns provider workload output evidence. This design extends the
[shared background-work design](background-work.md) and the
[Claude shell termination design](claude-background-shell-terminal-evidence.md).
Task persistence, WebSocket publication, and the desktop and phone viewers
consume the existing `background_work_output` contract without changes.

| Requirement | Design boundary |
| --- | --- |
| REQ-AGENTS-BACKGROUND-WORK-004 | Bounded retained output, truncation, duplicate-free reconnect |
| REQ-AGENTS-BACKGROUND-WORK-010 | Claude detached-shell output evidence (`010.10`) |

## Verified provider behavior

Claude Code writes the stdout and stderr of a backgrounded Bash command to
`<temp>/claude-<uid>/<project>/<native-session-id>/tasks/<task-id>.output`
(`%LOCALAPPDATA%\Temp\claude\...` on Windows). The file is a regular file owned
by the agent's user and grows while the command runs.

The Bash launch result names that file. Both launch variants use the same
sentence, and Kandev already receives it as the normalized shell output:

- `run_in_background: true`: `Command running in background with ID: <id>.
  Output is being written to: <path>`.
- Foreground timeout: `Command did not complete within its <n>s timeout and was
  moved to the background (ID: <id>). Output is being written to: <path>`.

Official `@agentclientprotocol/claude-agent-acp` 0.109.0 streams this file only
to ACP v2 clients (`background-output.js`) or AIR `asyncTasks` clients. Kandev
is an ACP v1, non-AIR client, so the bridge never forwards it. The rejected
[async-task decision](../../../decisions/2026-11-01-claude-async-task-lifecycle-capability.md)
still applies. The bridge's own `asyncTaskOutputFilePath` extracts the path from
the same Bash result text, so this design uses no less stable evidence than the
bridge does.

## Recognition boundary

Kandev tails a file only when every condition holds:

1. The adapter is Claude, `backgroundWorkEnabled` is on, and the connected
   bridge passes `verifiedClaudeTerminalBridge`. Only these shells get a
   workload ID and a terminal exit that stops reading.
2. The launch output contains `Output is being written to: <path>`. Read to the
   first line break, then end the path at the first
   `tasks[/\\]<name>.output` suffix followed by a sentence period, whitespace,
   or end of text. Clean the path before checking it.
3. The path is absolute, ends in `<grandparent>/tasks/<name>.output`, and
   `<grandparent>` equals the adapter's current native session ID.
4. `os.Stat` reports a regular file. A missing file is retried on the next
   poll, because Claude creates it lazily.

This file lies outside the task workspace. The session-ID and shape checks are
the authority for reading it. Any failure leaves the workload at
`Output: "none"`.

## Adapter flow

The file path validation and follower live in
`internal/agentctl/server/adapter/transport/acp/claude_shell_output.go`.
`adapter.go` owns follower lifecycle, `adapter_tools.go` connects verified
launch and terminal observations, and `dialect_claude_terminal.go` clears jobs
when the native session changes.

- **One poller per adapter.** The adapter keeps a map of followed jobs
  (`path`, `offset`, `lastGrowth`) and runs at most one poller goroutine, under
  `a.lifetimeCtx`, only while the map is non-empty. Jobs never get their own
  goroutine.
- **Start.** When the verified launch receives its workload ID and the path is
  valid, add the job to the map, start the poller if needed, and set its
  capability to `Output: "stream"`. Keep the job gated until its matching
  launch update has been queued; then activate it. Jobs that never activate
  still track file growth and expire after 30 minutes without growth. When 20
  jobs are already followed, the new job is not added and keeps
  `Output: "none"`.
- **Poll.** Every 2 s, `os.Stat` each followed file. Only a file whose size
  grew is read. Read at most the last 200 KiB of unread data, skipping older
  bytes and marking the chunk `Truncated` when needed. Align a skipped start
  and the emitted end to complete UTF-8 runes, then replace malformed input
  with U+FFFD before sending one `background_work_output` event. Keep the file
  cursor separate from the emitted stream offset: `Offset` counts bytes in the
  normalized UTF-8 chunks delivered to the task service. Emit progress without
  waiting for a newline. A split trailing rune remains unread and completes on
  a later poll. The task service's offset merge and 200 KiB retention handle
  the rest.
- **Stop following a job** when any of these happens:
  - its verified terminal exit arrives: read the final bounded unread window,
    send it before the terminal observation, then remove it;
  - its file has not grown for 30 minutes, so a job whose exit Kandev missed
    cannot be polled forever. Output already sent stays;
  - its file disappears after it existed, shrinks, or stops being a regular
    file;
  - the native session is replaced or the adapter closes, which drops the
    whole map.
- **Ordering.** Queue the launch update before activating its output follower.
  Chunks use `a.sendUpdate`, so the launch observation precedes its first
  chunk. A terminal observation follows the final read.
- **Lock scope.** A per-job delivery lock serializes each read through event
  delivery so output offsets and final chunks stay ordered. Release the job
  state lock before calling `a.sendUpdate`, because its activation hook reads
  the same job state.

The worst case is bounded: one goroutine and 20 `stat` calls every 2 s per
adapter, at most 200 KiB per file read, and every followed job leaves the map
within 30 minutes of its last output.

## Existing desktop and phone behavior

`workloadOutputUnavailableKey` already shows output whenever
`capabilities.output` is not `none` and `workload.output` is non-empty. The
existing search, auto-scroll, local clear, and truncation banner apply. The
phone detail uses the same retained output; the existing
`mobile-opencode-background-work.spec.ts` verifies that output is visible in
the phone detail. No web or locale change is required.

## Recovery and limits

- A restarted agentctl does not resume following. Process-end settlement
  (`010.1`) still ends the workload and the retained output stays.
- Unverified bridge versions and shells without a launch path keep today's
  `backgroundOutputUnavailable` state.
- Subagent async output and Monitor output files are out of scope; they keep
  their existing paths.
- A foreground command that Claude moves to the background after its timeout
  has no `run_in_background` input, so Kandev does not track it as a workload
  today. Tracking it is a separate recognition change; once tracked, it uses
  this tail unchanged because its result carries the same sentence.

## Verification

- Unit: path extraction for both launch sentences, including spaces in the
  path; clean repeated separators before checking shape; refuse relative,
  foreign-session, wrong-shape, and non-regular paths.
- Adapter: a verified 0.109.0 launch fixture with a real temp file advertises
  stream output, sends ordered chunks, and sends the final chunk before the
  terminal observation.
- Unit: emit progress without a newline, cap a large growth between polls,
  carry a split UTF-8 rune to the next poll, and keep offsets consistent after
  JSON replaces malformed UTF-8.
- Concurrency: block output delivery while activating the same job and verify
  activation completes before delivery resumes.
- Unit, with an injected clock: an unchanged file is not read; activated and
  unactivated jobs idle for 30 minutes leave the map; a 21st job keeps
  `Output: "none"`; the poller goroutine exits when the map empties.
- E2E: after the live detail opens, the mock-agent appends output and the
  viewer displays it before terminal frames are released; it also retains
  output after exit.
- Mobile regression: the existing phone detail test continues to verify that
  retained workload output is visible through the shared viewer contract.
