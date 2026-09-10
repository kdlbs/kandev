---
status: draft
system: platform
created: 2026-09-08
owners:
  - kandev
---

# Agent-accessible Kandev settings requirements

## Overview

The configuration assistant must find and change supported settings with the same authority and results as the user interface.
Platform owns this contract across settings domains. Each domain retains ownership of its data, permissions, validation, and execution behavior.

This delivery covers the core settings domains in requirement 006, not only agent profiles.
Agents can configure existing resources and use explicit lifecycle operations for supported setup tasks.
Every built-in Settings control receives a coverage classification. Discovery explains exceptions instead of hiding unsupported controls.
The existing document path and requirement IDs remain stable after this scope expansion.

## Terminology

- **Setting:** A named value within an existing resource.
- **Scope:** The resource and authority that determine a setting's meaning.
- **Support status:** Supported, read-only, interactive-only, client-local, or not-yet-supported, with a reason for each limitation.
- **Saved value:** The value stored for future use. It does not describe an active session's effective state.

## Requirements

### REQ-PLATFORM-AGENT-SETTINGS-PARITY-001: Discoverable coverage

**Intent:** The assistant can identify supported settings and explain limitations before it attempts a change.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-SETTINGS-PARITY-001.1:** Search shall match setting names, stable keys, descriptions, and curated aliases. Search shall not inspect saved values or secrets.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-001.2:** Each result shall identify its resource scope, support status, and Settings destination. Each limitation shall include a reason.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-001.3:** Detailed discovery shall provide types, allowed values, defaults or inheritance rules, change timing, required authority, and available operations.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-001.4:** Discovery shall report its covered domains. An unknown setting shall produce an explicit not-found result without a guessed operation.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-001.5:** Every built-in Settings control and editable domain field shall have an operation or explicit limitation. Nested fields and separate documents shall participate.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-001.6:** Search results shall name the setting, owning resource, mutation field, support status, and match reason without returning full resource schemas.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-001.7:** Detailed discovery shall identify dependent fields, replacement behavior, and required references. Resource context shall determine applicable choices.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-001.8:** Ambiguous search shall retain distinct results with their scopes. Search shall never select a resource or perform a mutation.

### REQ-PLATFORM-AGENT-SETTINGS-PARITY-002: Equivalent profile changes

**Intent:** A supported profile change has the same meaning through the assistant and the user interface.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-SETTINGS-PARITY-002.1:** Profile creation and updates shall accept the same supported fields, defaults, normalization, and domain validation as the user interface.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-002.2:** An omitted update field shall remain unchanged. Explicit false, empty strings, empty maps, and empty lists shall retain their documented meanings.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-002.3:** The assistant shall discover valid model-dependent choices for a selected profile context. Unavailable discovery shall report its state without inventing choices.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-002.4:** Rejected changes shall preserve saved state. Dependency conflicts and dynamic-profile version conflicts shall identify the same recovery choices as the user interface.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-002.5:** Each successful mutation shall publish one equivalent profile notification. Existing tool names and valid argument meanings shall remain compatible.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-002.6:** Saved-value reads shall identify the target profile and distinguish stored values from unresolved defaults and active-session state.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-002.7:** The configuration assistant shall use a compact update operation for adopted settings. New fields shall not enlarge its advertised argument schema.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-002.8:** The backend shall validate each update against the current registered resource contract before applying any change.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-002.9:** Invalid updates shall return structured field errors and a discovery reference. Unknown fields shall not be ignored or corrected automatically.

### REQ-PLATFORM-AGENT-SETTINGS-PARITY-003: Authority and sensitive values

**Intent:** Discovery and broader field coverage do not grant additional authority.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-SETTINGS-PARITY-003.1:** Each mutation shall enforce its existing user, workspace, resource, or organization authority at invocation. Discovery alone shall not authorize a mutation.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-003.2:** Missing or invalid identity shall not grant write access under enforced authentication. Auth-disabled installations shall preserve their existing single-user behavior.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-003.3:** New discovery and saved-value tools shall not reveal credential contents. They shall report redaction explicitly and preserve secret references as references.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-003.4:** A dependency override shall require the existing explicit confirmation flow. The assistant shall not automatically retry a conflict with an override.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-003.5:** Ordinary task, Office, and automation tool profiles shall not gain settings mutation authority from the catalog.

### REQ-PLATFORM-AGENT-SETTINGS-PARITY-004: Visible results and draft preservation

**Intent:** Users can observe assistant changes without losing unfinished edits.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-SETTINGS-PARITY-004.1:** An open, clean editor for an adopted setting shall show an accepted assistant change without reload. The same value shall remain after reload.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-004.2:** An external change shall not erase a dirty draft. The editor shall explain the conflict and require a fresh baseline before another save.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-004.3:** An acknowledgement of the editor's own save shall not create a false conflict or erase newer edits.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-004.4:** Desktop and phone shall provide the same result and recovery through existing settings routes and shared save controls.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-004.5:** New visible messages shall use the selected language. Keyboard and touch users shall reach the recovery action without horizontal page scrolling.

### REQ-PLATFORM-AGENT-SETTINGS-PARITY-005: Coverage regression detection

**Intent:** New fields and domains cannot silently recreate the settings coverage gap.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-SETTINGS-PARITY-005.1:** The coverage check shall fail for an editable adopted field or Settings control without an operation mapping or documented exclusion.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-005.2:** The checks shall detect schema, forwarding, persistence, error, and notification differences between supported interfaces.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-005.3:** Removing a catalog entry or adding an uncataloged field to an independent domain contract shall make the coverage check fail.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-005.4:** CI shall run registry completeness, generated-contract freshness, and search-quality checks for relevant changes.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-005.5:** A new adopted setting shall not add a tool or embed its schema in the compact tools' advertised definitions.

### REQ-PLATFORM-AGENT-SETTINGS-PARITY-006: Core settings coverage

**Intent:** Agents can configure the main Kandev settings domains without manual UI field entry.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-SETTINGS-PARITY-006.1:** Agents shall read and update portable user preferences, including task behavior, keybindings, appearance, terminal preferences, saved views, and utility defaults.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-006.2:** Agents shall read and update workflow settings and step settings, including prompts, profile bindings, events, and admission policies.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-006.3:** Agents shall read and update workspace defaults, repository settings, repository scripts, and repository-set settings.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-006.4:** Agents shall read and update agent definitions, executor settings, executor profiles, and environment definitions within existing editable contracts.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-006.5:** Agents shall read and update task configuration fields without bypassing active-session restrictions or triggering a lifecycle transition through a settings patch.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-006.6:** Agents shall read and update saved prompts, utility agents, and editor definitions. Built-in resource restrictions shall remain effective.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-006.7:** Agents shall read and update notification-provider settings and event subscriptions without disclosing saved credentials.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-006.8:** Agents shall update noncredential settings, presets, and watch configuration for built-in GitHub, GitLab, Azure DevOps, Jira, Linear, and Sentry integrations.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-006.9:** Agents shall read and update existing workspace automations and their triggers. Enabled-state changes shall report their scheduling effects.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-006.10:** Authorized agents shall update permitted runtime overrides and storage-maintenance policies. Environment locks, restart requirements, and destructive acknowledgements shall remain effective.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-006.11:** Delivery shall cover every eligible field in these domains. A not-yet-supported classification shall not satisfy an eligible field's delivery requirement.

### REQ-PLATFORM-AGENT-SETTINGS-PARITY-007: Explicit resource selection

**Intent:** An agent can find the correct target without guessing identifiers or changing another user's settings.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-SETTINGS-PARITY-007.1:** Resource lookup shall return bounded, paginated, authorized targets with names, resource types, and ownership context.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-007.2:** Personal settings shall resolve to the authenticated caller. Workspace-scoped settings shall require an explicit workspace or an authorized resource that determines it.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-007.3:** Contradictory selectors shall fail before mutation. Foreign resources shall not reveal their existence through lookup or detailed discovery.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-007.4:** Lookup shall preserve ambiguous candidates. Settings search shall find definitions rather than search saved values or select a target.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-007.5:** Required related-resource choices shall use authorized lookup. A returned choice shall not bypass validation during a later update.

### REQ-PLATFORM-AGENT-SETTINGS-PARITY-008: Domain invariants

**Intent:** Compact tools preserve domain rules for combined values, dependencies, and persistence.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-SETTINGS-PARITY-008.1:** Each update shall validate the resulting resource, including unchanged values and related-resource constraints, before committing the change.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-008.2:** Rejected updates shall not partially persist a settings patch. Required domain atomicity shall apply across every supported write interface.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-008.3:** Concurrent updates shall retain existing version checks and domain invariants. The assistant shall not report a settings revision as a concurrency guarantee without enforcement.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-008.4:** Successful updates shall preserve domain notifications, cache invalidation, and scheduling effects. Results shall distinguish saved state from active runtime state.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-008.5:** Maps, arrays, clearing, and nullable fields shall have discoverable update semantics. Redaction markers shall never overwrite saved values.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-008.6:** Missing providers and disabled features shall return explicit capability states. Their absence shall not remove static settings descriptions.

### REQ-PLATFORM-AGENT-SETTINGS-PARITY-009: Explicit boundaries

**Intent:** Agents can explain what requires a separate operation, user interaction, or deployment change.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-SETTINGS-PARITY-009.1:** Client-local preferences, startup-only values, secret contents, and plugin-owned settings shall have explicit limitations and recovery instructions.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-009.2:** Settings updates shall not install software, authenticate accounts, restart Kandev, restore backups, or execute immediate cleanup operations.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-009.3:** Create, delete, import, and reorder operations shall remain explicit. Existing valid lifecycle calls shall remain compatible.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-009.4:** Descriptions shall identify lifecycle tools needed for supported setup. New editable fields shall not enlarge their advertised settings payload definitions.
- **AC-PLATFORM-AGENT-SETTINGS-PARITY-009.5:** Agents shall not change authentication enablement, account authority, personal tokens, or their own MCP permissions through the generic settings tools.

## Out of scope

- Settings-file import, export, synchronization, or a new persistence source.
- New persistence for client-local preferences, deployment environment changes, and arbitrary startup-file writes.
- Plugin-authored settings, Office's separate organization-management model, and new runtime feature flags.
- Credential enrollment, account authority, authentication-mode changes, and secret-value retrieval.
- An unrestricted mutation tool, transactional multi-resource saves, or agent runtime behavior changes.
- Agent installation, login, duplication, and deletion parity beyond existing operations.
- Optimistic locking for all concrete-profile writes. Existing server concurrency semantics remain in force.

## Related contracts

- [Agent system](../../agents/README.md)
- [Dynamic provider choices](../../agents/requirements/dynamic-provider-options.md)
- [Settings navigation discovery](../../ui/requirements/settings-discovery.md)
- [System design](../system-design/agent-settings-parity.md)
- [Domain adoption design](../system-design/agent-settings-domains.md)
