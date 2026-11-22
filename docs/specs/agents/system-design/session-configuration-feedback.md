---
status: current
system: agents
created: 2026-11-20
requirements:
  - REQ-AGENTS-SESSION-CONFIG-FEEDBACK-001
---

# Session configuration feedback system design

Agents owns the evidence for a setting change. A picker reports its result
in the existing response. A slash command supplies its result with turn
completion. Task chat presents both as muted system messages.

| Acceptance criteria | Design section |
| --- | --- |
| AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.1–.3 | Picker responses |
| AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.4–.5 | Slash-command completion |
| AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.5–.7 | Feedback presentation |

## Picker responses

Keep the three session setter routes, their `ok` field, and their existing
error and persistence behavior. Add one optional `configuration_result` to the
response. It distinguishes the live outcome from preference persistence.

| Fact | Source |
| --- | --- |
| Effective value | Agentctl's existing mode result or the target value in the correlated ACP configuration response. Include the live execution identity. |
| Saved for launch | `applyIfSessionReady` skipped the live setter and Kandev wrote the requested override. |
| Rejected | Validation or the provider explicitly rejected this request. Use the existing error response for the reason. |
| Persistence failed | The live setter confirmed an effective value, but saving the preference failed. Preserve the effective value and show the save error. A deferred preference is never reported saved when its write fails. |

Every ACP agent uses the same picker path for the settings its session
exposes. Read the option and value from the normalized ACP data. Codex,
Claude, and other agents can supply different option IDs and labels.

Absent evidence means unconfirmed. This covers older peers, empty responses,
timeouts, and execution replacement. A local convergence event can echo
the requested value. Use the actual RPC response to report an effective
value. Use the existing adapter and lifecycle guards when forwarding it.
Fence picker responses, state writes, and retries by both the session's
current agentctl execution and the returned execution ID. A live response with
no execution identity is unconfirmed. Preserve strict errors on the existing
error-only agentctl setter methods; only the picker result path accepts
structured unconfirmed mode evidence. Treat an ambiguous timeout or disconnect
as unconfirmed, and preserve readable reasons for explicit rejection.

The chat uses its existing per-session request order and execution checks to
discard obsolete results. Keep the sequence for composed changes in
`applyModelConfiguration`. Commit effective values only from correlated
results. Stop dependent option writes when the base result is unconfirmed or
reports a different value. Preserve saved and unconfirmed steps in the result.
Keep a confirmed live value when preference persistence fails. Retry only an
explicitly rejected remaining option, and check the current action and
execution before every retry. The notice does not require a second
configuration state.
[Composed model contract](grouped-model-catalog.md)

## Slash-command completion

ACP slash command descriptions are free text. Use a validated command action
when available, as for Codex ACP's `/plan`. In the shared ACP adapter, a
plain `/model <id>` is a configuration attempt when the supplied ID matches
an advertised model ID exactly. A bare `/model`
can open a picker or list models and does not qualify as a setting change.
Claude ACP's model-switch hook illustrates the shared update path. It can
deliver the model observation asynchronously. [ACP slash commands](https://agentclientprotocol.com/protocol/v1/slash-commands)
· [Claude model-switch hook](https://github.com/agentclientprotocol/claude-agent-acp/blob/v0.100.0/src/acp-agent.ts#L12324-L12359)

Record the target and previous value on the current prompt. Use the existing
notification barrier and prompt generation to check the update at completion.
Require the same session and a change to the expected value. Use the
configuration guard and generation to detect overlapping setters. A
competing setter, unchanged value, or missing update leaves the outcome
unconfirmed. Late observations still update the picker through its
usual path. `end_turn` only closes the prompt.

Handle Copilot's extra arguments in its existing dialect. For
`/model auto hydrafusion`, `model=auto` can show that Copilot reported Auto
for the command. When that transition is correlated with the command, show a
brief receipt that says Copilot reported Auto and Fusion High was requested.
Keep the effective tier unconfirmed until the provider reports it explicitly.
An unrelated, unchanged, or ambiguous observation stays unconfirmed.
[Issue and Copilot probe](https://github.com/kdlbs/kandev/issues/5376)
· [ACP session configuration](https://agentclientprotocol.com/protocol/v1/session-config-options)
· [Command metadata](command-autocomplete.md)

Carry the optional command outcome through the existing numbered Ready
completion. `claimPromptCompletion` builds the Ready payload used for normal
successful prompts. It already holds the execution and turn identity.
Pass the outcome into settlement for that captured turn. Allow a live
complete-stream settlement for the same captured turn to use the same
helper. Treat a recognized configuration command as ineligible for the
existing dynamic empty-turn retry, since the command can change a setting.

Attach the outcome beside `had_output` in the live completion and its Core
conversation receipt only when the same turn's existing message read finds its
initiating user slash command. Keep the ordinary reply or recovery entry if
there is output. For a silent valid turn, the configuration outcome takes the
place of the generic hint. A turn without an owned user command has no receipt;
an unrecognized command uses the existing hint.

The receipt field is for live task chat. Recovered prompts, snapshots and
buffered replays only restore state and must omit the receipt. Use the existing
completed-turn identity to prevent duplicate notice emission independently of
the notice's dismissed or expired state. Preserve the meaning of `had_output`
and keep notice data out of conversation history and plugin turns.
[Empty-turn design](../../ui/system-design/empty-turn-notice.md)
· [Turn snapshot decision](../../../decisions/2026-07-18-turn-configuration-snapshots.md)

## Feedback presentation

Use the transcript's `StatusMessage` renderer for both feedback sources. Use
compact muted text in normal message flow. Use live setting and value labels
when available and the agent catalog labels before startup. Group confirmed
changes under “Changed” and saved settings under “Saved … for agent startup.”
List setting names and monospace values in both cases. Use red text for
unconfirmed picker clauses. Describe rejections, partial changes, and
uncertainty in text. Show the sanitized reason when the agent supplies one.
Dismiss and eligible Retry remain available in the row.

`usePickerConfigurationFeedback` forwards outcomes that pass the current
request and execution checks to one session-scoped collection. Group them by
the execution identity and newest accepted turn start when the picker action
begins. The execution closes the pre-launch group even when turn history is
still loading. Update the same row by setting ID, including the option IDs
from a composed Apply or Retry. Keep the row ID and position when another
outcome arrives. A late result belongs to its initiating group if its request
is still current. A group stops accepting Retry when its execution or turn
changes. An older or replayed start cannot reopen a closed group.

Keep picker rows in the session's in-memory UI state and project them as
local status messages when task chat builds its transcript. This keeps open
and closed rows available through history refreshes in the same view. A reload
starts with an empty collection. Remove the collection when the session is
removed. Keep Retry callbacks in UI state with the row they can update.

For a silent slash command, reuse the empty-turn notice's deterministic row
under the command. A positive receipt clears after five visible seconds.
A dismissible unconfirmed result stays in that turn. A duplicate completion
cannot recreate a dismissed or expired receipt. Reuse the existing late-output
filter if the agent's own reply arrives later.

Use polite announcements, preserve composer focus and the transcript's
reading position, and localize copy and accessible names. On phones, the
transcript remains the scroll owner above the keyboard. Text wraps and
Dismiss and Retry have 44 px touch targets. Render provider values and error
details as plain text.
