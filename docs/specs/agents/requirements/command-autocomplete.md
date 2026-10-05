---
status: active
system: agents
created: 2026-10-05
owners:
  - kandev
---

# Agent Command Autocomplete Requirements

## Overview

Agent bridges use different command names for skills. Codex ACP advertises `$retro`, while Claude and OpenCode advertise `retro`.
Users need readable entries, skill labels, mode state, and argument hints without changes to agent invocation.
Agents owns provider command identity and classification. The shared composer consumes these facts through its existing interaction contract.

## Terminology

- **Provider name:** The exact command name advertised by the active agent.
- **Display name:** The command name shown in autocomplete and the selected draft chip.
- **Identified skill:** A command whose provider contract positively identifies it as a skill.

## Requirements

### REQ-AGENTS-COMMAND-AUTOCOMPLETE-001: Skill presentation with preserved invocation

**Intent:** Users can identify and select skills without seeing provider syntax markers in their display names.

#### Acceptance criteria

- **AC-AGENTS-COMMAND-AUTOCOMPLETE-001.1:** When a provider advertises an identified skill with a dollar prefix, autocomplete shall show its name without that prefix. For `$retro`, the row shall show `/retro` and a localized `Skill` chip.
- **AC-AGENTS-COMMAND-AUTOCOMPLETE-001.2:** When command information does not distinguish skills from commands, autocomplete shall retain the advertised name without a skill chip. A description or a matching command name alone shall not establish skill identity.
- **AC-AGENTS-COMMAND-AUTOCOMPLETE-001.3:** When the user types `/retro`, autocomplete shall match the identified `$retro` skill. The existing `/$retro` query shall also match it. Entries with equal display names shall remain separate selectable entries.
- **AC-AGENTS-COMMAND-AUTOCOMPLETE-001.4:** When the user selects an identified skill, the draft chip shall show its clean display name. Submission, plain-text copying, draft restoration, and message recall shall preserve the provider command text and surrounding text. For Codex ACP `$retro`, that text remains `/$retro`.
- **AC-AGENTS-COMMAND-AUTOCOMPLETE-001.5:** Enter, Tab, pointer, and touch selection shall prepare an editable draft. Selection shall preserve focus and shall not send or queue a message. Explicit send and Escape shall retain the existing composer behavior.
- **AC-AGENTS-COMMAND-AUTOCOMPLETE-001.6:** Task chat and quick chat shall use the same presentation and invocation rules. Phone rows shall retain a minimum 44-pixel hit target and visible classification and state chips. Long descriptions shall not overlap the name or chips, and the menu shall remain inside the visible viewport.
- **AC-AGENTS-COMMAND-AUTOCOMPLETE-001.7:** When classification is absent or unsupported, existing command display and invocation shall remain available. A command from another provider with a literal dollar prefix shall retain that prefix unless its provider contract identifies it as a skill.

### REQ-AGENTS-COMMAND-AUTOCOMPLETE-002: Action meaning and confirmed mode state

**Intent:** Users can see what a supported command changes and what arguments it accepts before they send it.

#### Acceptance criteria

- **AC-AGENTS-COMMAND-AUTOCOMPLETE-002.1:** When verified provider metadata identifies a supported mode command, its autocomplete row shall show a localized `Mode` chip. A command name alone shall not establish mode behavior.
- **AC-AGENTS-COMMAND-AUTOCOMPLETE-002.2:** When confirmed session configuration matches the mode command's target value, the row shall show a localized `Active` chip. For Codex `/plan`, it shall describe turning plan mode off. Confirmed default mode shall omit `Active` and describe turning plan mode on.
- **AC-AGENTS-COMMAND-AUTOCOMPLETE-002.3:** When configuration is missing, unconfirmed, or outside the supported values, the row shall omit `Active` and use a neutral description. Draft selection and saved preferences shall not establish active mode state. Confirmed state from one session shall not affect another session's rows.
- **AC-AGENTS-COMMAND-AUTOCOMPLETE-002.4:** When a command advertises an argument hint, autocomplete shall show it as secondary text after its description. For `/goal`, the row shall retain its description and advertised hint without an invented category chip.
- **AC-AGENTS-COMMAND-AUTOCOMPLETE-002.5:** Metadata-derived chips and hints shall be noninteractive parts of the selectable row. Selection shall prepare a draft without changing session configuration or invoking an action. Sending shall retain the provider's command handling. Unsupported metadata shall leave the command selectable with its existing description and hint.

## Related contracts

- [Slash command composer selection](../../ui/requirements/slash-command-composer.md) owns draft selection and explicit sending.
- [Composer suggestion overlays](../../ui/requirements/composer-suggestion-overlays.md) owns popup geometry and touch interaction.
- [System design](../system-design/command-autocomplete.md) defines supported provider evidence and fallback behavior.

## Out of scope

- New skill discovery or filesystem scans for Claude and OpenCode.
- Changes to provider invocation syntax or automatic rewriting of manually typed text.
- New command execution, argument forms, or category filters.
- Native Codex app-server, passthrough terminals, the global command panel, and plan-editor slash commands.
- Changes to runtime versions or authentication.
