---
status: shipped
created: 2026-07-14
updated: 2026-07-16
owner: cfl
---

# ACP Shell Command Output

## Why

Agent chat shows that a shell command ran, but ACP agents encode terminal output and exit status in different fields. Users need the exact command and result status at a glance without large terminal transcripts consuming most of the conversation or being transferred when they are rarely inspected.

## What

- Shell tool calls from Codex, Claude, Auggie, and OpenCode normalize into the existing `shell_exec` message payload.
- Output received while a command is running is persisted on the existing tool message and appears in its expanded row before completion.
- Kandev advertises `_meta.terminal_output: true` in ACP client capabilities so agents such as Claude can return structured terminal output and exit metadata.
- Provider payloads normalize as follows:
  - Codex: append `_meta.terminal_output_delta.data`; on completion prefer `rawOutput.formatted_output` as the authoritative combined output and `_meta.terminal_exit.exit_code` or `rawOutput.exit_code` as the exit status.
  - Claude: replace the displayed output with `_meta.terminal_output.data` and read `_meta.terminal_exit.exit_code`. Plain final `rawOutput` remains a fallback for agents or versions that do not emit the extension.
  - OpenCode: replace the displayed output from cumulative text `content`; on completion prefer `rawOutput.output` and read `rawOutput.metadata.exit`.
  - Auggie: parse `rawOutput.output`, including its `<output>`, `<stderr>`, and `<return-code>` fields.
- A final authoritative output replaces the accumulated live output rather than appending it a second time. If a final update omits output or one explicit stream, the accumulated value for each omitted field remains visible.
- Exit-code precedence is `_meta.terminal_exit.exit_code`, provider-native structured exit fields, then Auggie's `<return-code>`. An absent or unparseable exit status remains unknown; it MUST NOT become exit `0`.
- Terminal text is treated as a combined stream unless an agent explicitly supplies separate stdout and stderr fields. Kandev does not infer stream separation from line ordering.
- Each normalized output text field is bounded to 256 KiB. When a field exceeds the bound, Kandev retains its most recent valid UTF-8 content and sets `truncated: true`.
- The full normalized command is always visible and wraps instead of truncating. The message content remains its fallback when a normalized command is absent. The working directory, when present, remains visible with the command.
- Stdout and stderr render in a separate disclosure that is collapsed by default for both running and completed commands. Running commands never auto-expand their output.
- Expanding the disclosure fetches the latest output snapshot on demand. A collapsed disclosure does not fetch or mount the output body.
- While an expanded command is running, Kandev refreshes its output with non-overlapping polling until the command becomes complete, failed, or cancelled. Polling stops immediately when the disclosure collapses or unmounts.
- The expanded disclosure shows combined output in a scrollable monospace region. On terminal completion it visibly shows `Exit code N` when known, or `Exit code unavailable` when unknown. Unknown is neutral, not success or failure.
- A known exit code of `0` is success. A known nonzero exit code is failure even when an agent reports ACP status `completed`. ACP `failed`/`error` remains failure independently of whether an exit code is available.
- ACP cancellation is terminal and preserves the transcript while showing `Exit code unavailable` when no exit was reported.
- Desktop and mobile chat expose the same output, truncation indication, and exit-status semantics.

## Data model

No table or migration is added. The existing persisted tool-message metadata carries:

```text
metadata.normalized.shell_exec.output
  exit_code  integer  optional; absent means unknown
  stdout     string   optional; combined terminal output unless streams are explicit
  stderr     string   optional; only populated from an explicit stderr field
  truncated  boolean  optional; true when either stored text field hit its bound
```

An explicit `exit_code: 0` is distinct from an absent exit code.

Normal client message projections replace the persisted output body with this summary at the same path:

```text
metadata.normalized.shell_exec.output
  exit_code     integer  optional; absent means unknown
  truncated     boolean  true when either persisted field hit its bound
  has_output    boolean  true when retained stdout or stderr is non-empty
  stdout_bytes  integer  UTF-8 byte count of retained stdout
  stderr_bytes  integer  UTF-8 byte count of retained stderr
```

`stdout` and `stderr` MUST be absent from message list, boot-state, and WebSocket message payloads. Projection does not mutate the persisted message metadata.

## API surface

This feature extends the existing `NormalizedPayload.shell_exec` contract. Agent stream events and persistence retain the full bounded output, while all browser-facing message projections carry only the output summary defined above. This applies to:

- `GET /api/v1/task-sessions/:session_id/messages` and its agent-session alias.
- The `message.list` WebSocket response.
- `session.message.added` and `session.message.updated` WebSocket notifications.
- Task-route boot state under `initialState.messages`.

The full output snapshot is available from:

```http
GET /api/v1/task-sessions/:session_id/messages/:message_id/shell-output
```

Response `200`:

```json
{
  "message_id": "message-id",
  "status": "running",
  "updated_at": "2026-07-16T12:00:00Z",
  "output": {
    "exit_code": 0,
    "stdout": "retained stdout",
    "stderr": "retained stderr",
    "truncated": false
  }
}
```

`exit_code`, `stdout`, and `stderr` are omitted when unavailable. The endpoint returns `404` when the message does not exist in the path session or is not a normalized shell-execution message; this avoids exposing cross-session message existence. It returns `200` with an empty `output` object when the shell command is valid but has not emitted output yet.

The frontend issues one immediate request when the disclosure opens. For a running response it schedules the next request only after the previous request settles, starting at one second and backing off on consecutive failures to at most five seconds. It keeps the latest successful snapshot during a transient failure and resumes the base interval after success. A terminal response stops recurring polling. When the normal message projection transitions to a terminal status while the disclosure is open, the frontend aborts any in-flight poll, fetches one final snapshot, and then stops. Collapse or unmount stops polling and aborts any in-flight request without a final fetch. This behavior uses component-local hook state, not the session message store.

Decision: [ADR-0042](../../decisions/0042-project-shell-output-and-fetch-on-demand.md).

ACP initialization includes:

```json
{
  "clientCapabilities": {
    "_meta": { "terminal_output": true }
  }
}
```

The extension is additive: agents that ignore it continue through their current `rawOutput` or `content` fallback.

## Failure modes

- Malformed provider output is retained as combined terminal text when possible; malformed exit metadata remains unknown.
- A statusless update with recognized terminal output is treated as an in-progress tool update so it is not dropped by the existing persistence path.
- Repeated cumulative output replaces the previous cumulative value; delta output appends once. Final aggregate output replaces either form and cannot duplicate the visible text.
- Output exceeding the bound is truncated deterministically and remains valid UTF-8.
- Unknown exit status never renders a success check or a failure cross solely because the value is absent.
- ACP `failed` and `cancelled` statuses terminate active-call tracking even when no exit code is available.
- A terminal tool update received after its originating turn settled reconciles the existing tool message without opening a new turn or changing session or task state.
- A failed on-demand request leaves the disclosure open, retains the last successful snapshot, and shows an output-unavailable state until a retry succeeds. It never falls back to a body from the normal message payload.
- Poll responses that arrive after collapse, unmount, or a newer request, including the final terminal snapshot request, are ignored.

## Persistence guarantees

Live output updates use the existing tool-message update path and are persisted with message metadata. The latest bounded output and final exit status survive reloads and are read by the on-demand endpoint. The summary projection is computed at delivery time; it is not separately persisted. In-memory per-tool accumulation is discarded when the tool reaches a terminal state, the prompt is swept, or the adapter stops.

## Scenarios

- **GIVEN** Codex emits two `terminal_output_delta` updates followed by `formatted_output` and exit `4`, **WHEN** the command row updates, **THEN** output appears during execution, the final text contains each line once, and completion shows `Exit code 4` as failure.
- **GIVEN** Claude receives the advertised terminal-output capability and emits `terminal_output` followed by `terminal_exit`, **WHEN** the command completes, **THEN** its output is visible and the exact exit code is shown.
- **GIVEN** OpenCode emits cumulative `content` values followed by `rawOutput.metadata.exit: 7` while ACP status is `completed`, **WHEN** the command completes, **THEN** the latest output is not duplicated and the row shows `Exit code 7` as failure.
- **GIVEN** Auggie returns XML-like output with `<return-code>0</return-code>`, **WHEN** the command completes, **THEN** the parsed output is visible and the row shows `Exit code 0` as success.
- **GIVEN** an agent returns plain output with no structured or embedded exit status, **WHEN** the command completes, **THEN** the output is visible and the row shows `Exit code unavailable` with neutral status.
- **GIVEN** a command is cancelled without an exit code, **WHEN** its terminal update is persisted, **THEN** the transcript remains expandable and shows `Exit code unavailable`.
- **GIVEN** a command's turn has settled and the provider later emits a terminal tool update, **WHEN** the update is persisted, **THEN** the existing transcript is reconciled without creating a turn, waking the session or task, or producing an empty-output warning.
- **GIVEN** terminal output exceeds 256 KiB, **WHEN** live and final updates are normalized, **THEN** stored output remains within the bound, keeps the newest valid UTF-8 text, and the expanded row indicates truncation.
- **GIVEN** a persisted shell message with a long command, **WHEN** chat opens on desktop or mobile, **THEN** the complete command wraps visibly while its output disclosure remains collapsed.
- **GIVEN** a collapsed shell command with persisted output, **WHEN** task boot state, message REST/WS lists, and live message notifications reach the browser, **THEN** they contain byte counts and result metadata but no stdout or stderr body, and no output endpoint request occurs.
- **GIVEN** a persisted completed shell message, **WHEN** the user expands its output disclosure, **THEN** exactly one on-demand snapshot is fetched and its output, truncation, and exit status are readable without overlapping adjacent chat content.
- **GIVEN** a running shell message with its disclosure expanded, **WHEN** output changes and the command later completes, **THEN** bounded non-overlapping polling refreshes the transcript and stops after the terminal snapshot.
- **GIVEN** an expanded running shell message, **WHEN** the user collapses it or navigates away, **THEN** scheduled polling stops and an in-flight request cannot update the unmounted disclosure.
- **GIVEN** an output request fails after a prior snapshot succeeded, **WHEN** the disclosure remains expanded, **THEN** the prior transcript stays visible, an unavailable state is shown, and capped retry polling can recover.

## Out of scope

- Reconstructing separate stdout and stderr streams when the agent sends only combined output.
- Fixing provider-side output loss before an ACP frame reaches Kandev.
- Adding a terminal emulator, ANSI replay, command re-run action, download action, or searchable output viewer to chat.
- Changing non-ACP adapters or the standalone terminal panel.
- Moving shell output into a new table, object store, or streaming/chunked output protocol.

## Implementation plan

See [the implementation plan](../../plans/acp-shell-command-output/plan.md).
