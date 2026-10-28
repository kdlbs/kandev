---
status: active
system: agents
created: 2026-10-28
owners:
  - kandev
---

# Native Prompt Suggestion Capability Requirements

## Overview

Kandev must determine native suggestion support separately from a user's request
to use it. A provider namespace alone does not establish feature support.

Agents owns this contract because it owns provider capabilities and adapter
normalization. UI retains the independent
[composer interaction contract](../../ui/requirements/composer-prompt-suggestions.md).
Tasks retains session state, originating turns, and conditional writes.

## Terminology

- **Provider support:** Evidence that the connected agent integration implements
  native next-message suggestions. It does not guarantee a suggestion for every turn.
- **Request:** The launch-time choice derived from user preference and session scope.
- **Effective enablement:** Provider support and a request to use it for this session.
- **Compatibility evidence:** A documented, verified implementation/version contract
  used when the provider has no explicit feature advertisement.

## Requirements

### REQ-AGENTS-NATIVE-SUGGEST-001: Feature-specific provider evidence

**Intent:** Enable native suggestions only when the integration supplies sufficient evidence.

#### Acceptance criteria

- **AC-AGENTS-NATIVE-SUGGEST-001.1:** When a bridge explicitly declares the
  supported native-suggestion contract, Kandev shall recognize support independently
  of its configured agent name, reported name, and selected model.
- **AC-AGENTS-NATIVE-SUGGEST-001.2:** When an explicit declaration is absent,
  Kandev shall preserve native support for the verified, pinned Claude bridge
  through a documented compatibility rule. That rule shall identify its evidence
  and version limits.
- **AC-AGENTS-NATIVE-SUGGEST-001.3:** When evidence is unknown, empty, malformed,
  unrelated, or insufficient, Kandev shall report no support and send no
  suggestion-enabling request. A vendor namespace or another capability alone
  shall not qualify.
- **AC-AGENTS-NATIVE-SUGGEST-001.4:** When an explicit declaration denies support
  or has an invalid value, Kandev shall not override it with compatibility evidence.
- **AC-AGENTS-NATIVE-SUGGEST-001.5:** A model name shall never establish native
  support. Unsupported OpenCode and Codex integrations shall remain unsupported
  when they use a Claude model.
- **AC-AGENTS-NATIVE-SUGGEST-001.6:** Documentation shall distinguish existing
  upstream protocol evidence from a new extension that requires provider adoption.
  Kandev shall not describe that extension as standard ACP.

### REQ-AGENTS-NATIVE-SUGGEST-002: Independent support and session enablement

**Intent:** Give Kandev consumers a generic capability without changing the accepted opt-in behavior.

#### Acceptance criteria

- **AC-AGENTS-NATIVE-SUGGEST-002.1:** Runtime consumers shall receive provider-neutral
  support and effective enablement. Lifecycle, orchestration, and composer decisions
  shall not inspect provider names, model names, or vendor metadata for this feature.
- **AC-AGENTS-NATIVE-SUGGEST-002.2:** When a supported provider starts with the
  preference off, support shall remain true and effective enablement shall be false.
  Kandev shall not request generation or admit suggestion notifications.
- **AC-AGENTS-NATIVE-SUGGEST-002.3:** When the preference is on for an eligible
  interactive task or Quick Chat session, effective enablement shall require provider
  support. Unsupported sessions shall retain normal composer behavior.
- **AC-AGENTS-NATIVE-SUGGEST-002.4:** Office, automation, utility, configuration,
  coordinator, task-creation, and passthrough sessions shall not request suggestions.
  The preference shall remain default off, without a separate runtime flag.
- **AC-AGENTS-NATIVE-SUGGEST-002.5:** Existing consumers of
  `supports_prompt_suggestions` shall retain its effective-enablement meaning.
  A new support-only value shall not enable their composer.
- **AC-AGENTS-NATIVE-SUGGEST-002.6:** Saving the preference shall hide or allow
  suggestions immediately through existing UI rules. Provider configuration shall
  follow the preference at the next launch or resume, without requests on typing.

### REQ-AGENTS-NATIVE-SUGGEST-003: Session continuity and normalized delivery

**Intent:** Keep capability changes from weakening session and suggestion-origin safeguards.

#### Acceptance criteria

- **AC-AGENTS-NATIVE-SUGGEST-003.1:** New, loaded, resumed, reset, and recreated
  sessions shall use the support evidence and request for their current connection.
  A replacement connection shall not inherit an earlier connection's support.
- **AC-AGENTS-NATIVE-SUGGEST-003.2:** When support or requested enablement is lost
  on restart or resume, Kandev shall clear effective support and the native source
  marker. A stored suggestion shall then remain hidden, including after reload.
- **AC-AGENTS-NATIVE-SUGGEST-003.3:** Normalized notifications shall retain their
  originating prompt generation. Unknown, ambiguous, overlapping, duplicate, and
  active-prompt origins shall retain the existing rejection behavior.
- **AC-AGENTS-NATIVE-SUGGEST-003.4:** Unknown, foreign, or stale originating turns
  shall cause no suggestion write or publication. Accepted writes shall retain
  the waiting-for-input state guard.
- **AC-AGENTS-NATIVE-SUGGEST-003.5:** Another native provider shall integrate
  through its adapter's generic capability and suggestion event. Its integration
  shall not require provider branches in composer business logic.

## Related contracts

Desktop and phone acceptance, exact literal text, recovery blocking, empty-draft
visibility, dismissal, and submission remain governed by
`REQ-UI-PROMPT-SUGGEST-001` through `REQ-UI-PROMPT-SUGGEST-004`.
This feature changes eligibility evidence, without a new interaction or layout.

## Out of scope

- Additional provider implementations, model requests, or generated replacements.
- A provider registry, speculative adapter framework, or persistent capability cache.
- Runtime feature flags, new preferences, and per-keystroke completion.
- Publishing upstream issues or changes during this design phase.

## Implementation plans

- [Native capability normalization](../../../plans/native-prompt-suggestion-capability/plan.md)

