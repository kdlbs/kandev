---
status: shipped
created: 2026-07-14
owner: cfl
---

# ACP Shell Command Output

## Why

Agent chat shows that a shell command ran, but ACP agents encode terminal output and exit status in different fields. Users need the expandable command row to preserve the actual terminal output and distinguish success, failure, and an unavailable exit status without knowing which agent produced the command.

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
- The existing expandable command row shows combined output in a scrollable monospace region. On terminal completion it visibly shows `Exit code N` when known, or `Exit code unavailable` when unknown. Unknown is neutral, not success or failure.
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

## API surface

This feature extends the existing `NormalizedPayload.shell_exec` JSON contract carried by agent stream events and task messages. It adds no HTTP route, WebSocket event type, or frontend store slice.

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

## Persistence guarantees

Live output updates use the existing tool-message update path and are persisted with message metadata. The latest bounded output and final exit status survive reloads. In-memory per-tool accumulation is discarded when the tool reaches a terminal state, the prompt is swept, or the adapter stops.

## Scenarios

- **GIVEN** Codex emits two `terminal_output_delta` updates followed by `formatted_output` and exit `4`, **WHEN** the command row updates, **THEN** output appears during execution, the final text contains each line once, and completion shows `Exit code 4` as failure.
- **GIVEN** Claude receives the advertised terminal-output capability and emits `terminal_output` followed by `terminal_exit`, **WHEN** the command completes, **THEN** its output is visible and the exact exit code is shown.
- **GIVEN** OpenCode emits cumulative `content` values followed by `rawOutput.metadata.exit: 7` while ACP status is `completed`, **WHEN** the command completes, **THEN** the latest output is not duplicated and the row shows `Exit code 7` as failure.
- **GIVEN** Auggie returns XML-like output with `<return-code>0</return-code>`, **WHEN** the command completes, **THEN** the parsed output is visible and the row shows `Exit code 0` as success.
- **GIVEN** an agent returns plain output with no structured or embedded exit status, **WHEN** the command completes, **THEN** the output is visible and the row shows `Exit code unavailable` with neutral status.
- **GIVEN** a command is cancelled without an exit code, **WHEN** its terminal update is persisted, **THEN** the transcript remains expandable and shows `Exit code unavailable`.
- **GIVEN** a command's turn has settled and the provider later emits a terminal tool update, **WHEN** the update is persisted, **THEN** the existing transcript is reconciled without creating a turn, waking the session or task, or producing an empty-output warning.
- **GIVEN** terminal output exceeds 256 KiB, **WHEN** live and final updates are normalized, **THEN** stored output remains within the bound, keeps the newest valid UTF-8 text, and the expanded row indicates truncation.
- **GIVEN** a persisted completed shell message, **WHEN** chat is opened on desktop or mobile and the row is expanded, **THEN** its output and exit status are readable without overlapping adjacent chat content.

## Out of scope

- Reconstructing separate stdout and stderr streams when the agent sends only combined output.
- Fixing provider-side output loss before an ACP frame reaches Kandev.
- Adding a terminal emulator, ANSI replay, command re-run action, download action, or searchable output viewer to chat.
- Changing non-ACP adapters or the standalone terminal panel.

## Implementation plan

See [the implementation plan](../../plans/acp-shell-command-output/plan.md).
