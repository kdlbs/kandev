---
status: draft
system: agents
created: 2026-11-01
owners:
  - kandev
---

# ACP Session Events Requirements

## Overview

ACP schema 1.28 lets a client receive explicit context-compaction and notice
events instead of inferring them from text or usage changes. Kandev users can see
when an agent compacts its context and can tell provider warnings apart from the
agent's reply. The agents system owns this contract because it interprets provider
wire behavior. Task storage and chat consume the normalized result.

## Terminology

- **Compaction entry:** One chat timeline row for one provider-identified
  compaction, updated in place as it progresses.
- **Notice:** A provider message for the user that is not part of the agent's
  reply.
- **Reporting agent:** An agent that sends the corresponding ACP event after
  Kandev advertises support for it.

## Requirements

### REQ-AGENTS-ACP-SESSION-EVENTS-001: Compatible ACP schema 1.28 bindings

**Intent:** Kandev uses the published fork release without changing existing agent
behavior for agents that do not send the new events.

#### Acceptance criteria

- **AC-AGENTS-ACP-SESSION-EVENTS-001.1:** The backend shall resolve the ACP SDK to
  the merged fork `main` revision of schema 1.28, not to a pull-request revision.
- **AC-AGENTS-ACP-SESSION-EVENTS-001.2:** Remote prompt-error origin evidence shall
  remain available, so a peer-returned error stays distinguishable from a local
  send, decode, deadline, or disconnect failure.
- **AC-AGENTS-ACP-SESSION-EVENTS-001.3:** A config option with a missing or empty
  name that the SDK filters from a response shall not create a session option, and
  Kandev shall report the existing unsupported-model outcome when no usable model
  option remains.

### REQ-AGENTS-ACP-SESSION-EVENTS-002: Context compaction timeline

**Intent:** Users can see when context was compacted, what was retained, and
whether compaction succeeded.

**User story:** As a user following an agent session, I want a compaction shown in
the conversation, so that I can explain a change in the agent's retained context.

#### Acceptance criteria

- **AC-AGENTS-ACP-SESSION-EVENTS-002.1:** When a reporting agent starts a
  compaction, the chat shall show one in-progress compaction entry at that timeline
  position.
- **AC-AGENTS-ACP-SESSION-EVENTS-002.2:** When later events carry the same compaction
  identity, the system shall update that entry in place and shall not create a second
  entry.
- **AC-AGENTS-ACP-SESSION-EVENTS-002.3:** When compaction completes, fails, or is
  cancelled, the entry shall show that outcome. A failed entry shall show the reported
  error when one exists.
- **AC-AGENTS-ACP-SESSION-EVENTS-002.4:** When a summary is reported, whether whole or
  streamed, the entry shall offer it as expandable content. An omitted summary shall
  keep the stored value, and a cleared summary shall remove it.
- **AC-AGENTS-ACP-SESSION-EVENTS-002.5:** When the provider reports a trigger or
  before and after token counts, the entry shall show them. When it does not, the entry
  shall omit them without showing an empty or zero value.
- **AC-AGENTS-ACP-SESSION-EVENTS-002.6:** When a session history is replayed during
  load, replayed compaction events shall not create or change an entry.
- **AC-AGENTS-ACP-SESSION-EVENTS-002.7:** When a page is refreshed or the backend
  restarts, a persisted entry shall retain its last reported state. An entry still in
  progress after its turn ends shall not appear active.
- **AC-AGENTS-ACP-SESSION-EVENTS-002.8:** The existing compaction count and its
  usage-drop derivation shall not change.
- **AC-AGENTS-ACP-SESSION-EVENTS-002.9:** Agents that report compaction as ordinary
  text or tool output shall keep their existing presentation.
- **AC-AGENTS-ACP-SESSION-EVENTS-002.10:** The entry shall be readable and its summary
  operable on desktop and phone, including by keyboard and touch.

### REQ-AGENTS-ACP-SESSION-EVENTS-003: Provider notices

**Intent:** Users can notice provider warnings without mistaking them for the agent's
reply or for a failed session.

#### Acceptance criteria

- **AC-AGENTS-ACP-SESSION-EVENTS-003.1:** When an agent reports a notice, the chat shall
  show a status row with its title, and its description when one exists.
- **AC-AGENTS-ACP-SESSION-EVENTS-003.2:** The row shall distinguish info, warning, and
  error severity. An unknown severity shall render as info.
- **AC-AGENTS-ACP-SESSION-EVENTS-003.3:** A notice, including one with error severity,
  shall not change session state, end a turn, or start error recovery.
- **AC-AGENTS-ACP-SESSION-EVENTS-003.4:** Provider notice text shall render as plain
  text and shall not be interpreted as markup.
- **AC-AGENTS-ACP-SESSION-EVENTS-003.5:** A notice delivered once by the provider shall
  produce one row after a reconnect or restart, and notices replayed during session load
  shall not create rows.

### REQ-AGENTS-ACP-SESSION-EVENTS-004: Scoped capability advertisement

**Intent:** Kandev asks for new events only where it can present them.

#### Acceptance criteria

- **AC-AGENTS-ACP-SESSION-EVENTS-004.1:** Interactive agent sessions shall advertise
  compaction and notice support to every ACP agent.
- **AC-AGENTS-ACP-SESSION-EVENTS-004.2:** Probe and utility inference sessions shall not
  advertise either capability, because they do not present these events.
- **AC-AGENTS-ACP-SESSION-EVENTS-004.3:** Kandev shall not advertise subagent-session
  support or native-subagent extensions.

### REQ-AGENTS-ACP-SESSION-EVENTS-005: Debug tooling for ACP capabilities

**Intent:** Developers can reproduce capability-gated agent behavior without a
temporary wire proxy.

#### Acceptance criteria

- **AC-AGENTS-ACP-SESSION-EVENTS-005.1:** When a developer supplies a JSON object of
  client capabilities, `acpdbg` shall merge it into the `initialize` request and record
  the sent frame.
- **AC-AGENTS-ACP-SESSION-EVENTS-005.2:** When the supplied value is not a JSON object,
  `acpdbg` shall fail before starting the agent and explain the expected value.
- **AC-AGENTS-ACP-SESSION-EVENTS-005.3:** When a developer selects a model and the agent
  advertises a model config option, `acpdbg` shall select the model through that option.
  Otherwise it shall use the legacy model request.

## Out of scope

- Native child sessions, inter-session messages, client-supplied system prompts, and MCP
  over ACP. No pinned agent supports them in a usable way.
- Compaction or notice presentation outside the chat timeline, such as toasts or task
  list badges.
- Parsing provider text to synthesize compaction or notice entries.
- Starting a compaction from the Kandev UI.
