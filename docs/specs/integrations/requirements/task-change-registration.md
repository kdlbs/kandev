---
status: draft
system: integrations
created: 2026-10-09
owners:
  - kandev
---

# Task change request registration requirements

## Overview

Selected PR/MR references, including compatible plugin providers, must become durable task associations when users submit them. Agents must also register relevant work expressed in plain text.
Integrations owns this outcome because it owns contribution identity, associations, provider capabilities, and their visible state.
Tasks retains message admission, session identity, and queue ownership.

This is a proposed extension of [task change request management](task-change-link-mcp.md), not another association system.
Its independent contract is admission-driven registration and its feedback. Existing management, replacement, automation, and authorization guarantees remain authoritative.
The URL/default-target extension below will amend management criteria 001.1 and 003.1 during implementation. Until then, their current explicit-field contract remains active.

## Terms

- **Selected reference:** A structured reference chosen through the mention picker.
- **Context only:** A reference that supplies background and requests no association.
- **Accepted submission:** A durable user message, queue admission, or successful queue-edit save.
- **Observation-only association:** A link that adds visibility without granting execution or comparison authority.
- **Intentional relink:** A new user selection submitted after unlink, or an explicit Link again action.

## Requirements

### REQ-INTEGRATIONS-CHANGE-REGISTRATION-001: Link selected references at acceptance

**Intent:** A submitted work reference appears in the task's linked contributions.

#### Acceptance criteria

- **AC-INTEGRATIONS-CHANGE-REGISTRATION-001.1:** A supported selected PR/MR defaults to linking. Before Send, its control states “Link on send” and offers “Context only”.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-001.2:** Initial and later direct messages request association only after durable message acceptance. Queue submissions request association after durable queue admission, including automatic folds.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-001.3:** Draft edits, cancelled draft submission, opening a queue editor, and cancelling that editor create no association. A rejected admission creates none.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-001.4:** A committed queue edit requests association for newly selected references or an explicit change from context-only to linking. Removing and reselecting the same reference counts as a new selection, even when its final content matches. An unchanged reference does not request relinking. Cancel and replay create no fresh request.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-001.5:** Queue removal, clearing, dispatch, message deletion, or session cancellation after acceptance does not unlink an established association. Removing a reference in a saved edit does not unlink it. These outcomes remain visible in the reference controls.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-001.6:** Context-only references and historical references without explicit linking intent create no association. Pasted URLs and ambiguous plain “#123” text create no automatic association.

### REQ-INTEGRATIONS-CHANGE-REGISTRATION-002: Resolve exact supported identity

**Intent:** A task links the intended contribution without changing its workspace.

#### Acceptance criteria

- **AC-INTEGRATIONS-CHANGE-REGISTRATION-002.1:** Identity includes provider, verified host, connection scope, immutable repository identity, and change-request identifier. Built-in PR/MR numbers remain positive integers. Equal numbers in different repositories, forks, or hosts remain distinct. Repository renames preserve identity. Reused paths never inherit another repository's links or unlink suppression.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-002.2:** GitHub, configured GitLab, and compatible plugin references link only through a unique repository identity supported by the current task association contract. Missing, foreign, detached, or ambiguous repositories stay as context with a reason and an explicit link action. Resolution never selects the first attached repository, attaches a repository, or changes checkout.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-002.3:** Providers without the required registration capability, including built-in Azure DevOps for this feature, remain readable context with an explanation. Unavailable credentials and failed provider reads show failure without a false success or an empty-success linked set.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-002.4:** Wrong-workspace, forged provider identity, unsafe destinations, and unauthorized callers fail before association. Provider titles and fetched text remain untrusted data.

### REQ-INTEGRATIONS-CHANGE-REGISTRATION-003: Register agent work through existing tools

**Intent:** PR work reaches the task list even when the user supplies plain text.

#### Acceptance criteria

- **AC-INTEGRATIONS-CHANGE-REGISTRATION-003.1:** Shared guidance instructs agents to register each created, actively reviewed, fixed, or updated PR/MR when that work starts. It includes all relevant stack layers, excludes background-only references, and requires a final linked-set check.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-003.2:** Existing management accepts a full supported URL and defaults an omitted target to the authenticated calling task. Existing explicit identity calls remain valid. Conflicting URL and explicit identity fields fail without mutation.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-003.3:** Guidance uses `manage_task_change_request_kandev` and `get_task_change_requests_kandev`. A CLI/API creation requires registration of the returned URL. A failed association must be reported accurately; it must not be represented as a linked PR.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-003.4:** Caller, workspace, session, and origin identity come from authenticated server context. An optional legacy target task remains a requested target subject to current reach checks, never caller authority.

### REQ-INTEGRATIONS-CHANGE-REGISTRATION-004: Preserve links and deliberate unlink

**Intent:** Replay cannot override a user's removal or replace sibling links.

#### Acceptance criteria

- **AC-INTEGRATIONS-CHANGE-REGISTRATION-004.1:** Retries, concurrent sessions, reconnect, queue folds, and message replay produce at most one active association per exact task/contribution identity. Linking another contribution preserves every sibling link.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-004.2:** Explicit unlink suppresses stale admitted work, ordinary agent registration, and automatic re-detection after restart. An in-flight association cannot restore a link after a later unlink commits.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-004.3:** A new selected reference submitted after unlink, or an explicit Link again action, can restore that exact association. Old submission replay and ordinary Retry cannot restore it. Unavailable or stale provider state prevents restoration and offers a fresh Link again action after recovery.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-004.4:** Links identify mention selection versus agent registration and the available originating message/session. Message deletion leaves a non-navigable origin and preserves the link. Provenance never grants authority or exposes another workspace.

### REQ-INTEGRATIONS-CHANGE-REGISTRATION-005: Separate association from execution

**Intent:** Adding visibility preserves user control of work and comparison.

#### Acceptance criteria

- **AC-INTEGRATIONS-CHANGE-REGISTRATION-005.1:** New links permit ordinary status observation but enable no wake-on-change, auto-fix, merge, lifecycle prompt, workflow advancement, or broader agent permission. Existing explicitly authorized automation remains unchanged on already-linked contributions.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-005.2:** Initial association, later sync, retry, and re-detection preserve checkout, comparison scope, manual base overrides, and session base SHAs. They create no comparison remote or fetch for comparison.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-005.3:** Registration and retry preserve question barriers, session ownership, cancellation, queue leases, and workspace permissions. They never start or resume an agent to complete registration.

### REQ-INTEGRATIONS-CHANGE-REGISTRATION-006: Show truthful, accessible state

**Intent:** Users can see and recover association results without losing messages.

#### Acceptance criteria

- **AC-INTEGRATIONS-CHANGE-REGISTRATION-006.1:** Submitted references show pending, linked, context-only, suppressed/unlinked, failed, or uncertain state. Unknown mutation outcomes remain distinct from confirmed failures. “Linked PR #123” or “Linked MR !123” appears only after persistence. One reference failure preserves the accepted message and other successful links.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-006.2:** Existing task badges, lists, navigation, and unlink controls reflect committed updates across sessions without reload. Reconnect and reload recover current state; stale success must not reinsert a detached link.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-006.3:** Retry targets only the failed association, preserves the message, and shows structured recovery guidance. Unsupported targets offer an explanatory link control and Open reference; they never silently change repositories.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-006.4:** Desktop and phone provide equivalent context-only, status, retry, provenance, and unlink outcomes. Controls support keyboard use, focus return, screen-reader names and announcements, localization, and phone touch targets of at least 44 pixels. Long titles and translated text cause no page overflow.

### REQ-INTEGRATIONS-CHANGE-REGISTRATION-007: Register plugin-provided change requests

**Intent:** Compatible remote-repository plugins provide the same association outcome as built-in providers.

#### Acceptance criteria

- **AC-INTEGRATIONS-CHANGE-REGISTRATION-007.1:** An active plugin with authorized reference search can supply PR/MR results to the existing `#` autocomplete. Repository discovery alone does not imply reference search or linking support.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-007.2:** A plugin with compatible backend registration capability supports default selected-reference linking, context-only selection, and initial/direct/queued admission through the same user controls. Search-only and older plugins show context-only capability with an explanation.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-007.3:** The existing shared management/read tools support capable plugin providers, including URL input and current-task default. No provider-specific registration MCP tool is required. A plugin-only task receives the applicable shared guidance and tools without gaining unsupported automation capabilities.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-007.4:** Plugin identities distinguish host, connection scope, immutable repository, and change identifier. Equal numbers across Bitbucket Cloud/Data Center, Data Center context roots, and Forgejo installations cannot collide. Conflicting provider matches fail without mutation or repository attachment.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-007.5:** Plugin registration preserves observation-only behavior, sibling links, provenance, replay safety, and explicit unlink across restart. If association resolution fails, an otherwise authorized message remains accepted with a recoverable registration error. Recovery never gains permission to restore a detached link. Lost mutation replies produce uncertain state until reconciliation; they never cause a blind duplicate or automatic restoration.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-007.6:** Disable, uninstall, upgrade, revoked access, or missing capability prevents new dispatch to stale plugin instances. Accepted messages remain intact. Reads distinguish provider unavailability from an empty linked set; unavailable links retain truthful status and cannot claim successful unlink.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-007.7:** Plugin success updates the existing registered review-provider lists, task indicators, counts, navigation, and unlink controls on desktop and phone. Feedback and provenance use shared localized controls with keyboard and screen-reader access.
- **AC-INTEGRATIONS-CHANGE-REGISTRATION-007.8:** Capability-compatible and older plugins can coexist. Existing manual link/create/unlink flows remain available under their established contracts. Plugin authors receive versioned adoption guidance and conformance tests; provider-specific logic stays outside the host.

## Exclusions

New association stores, arbitrary repository attachment, built-in Azure neutral-tool expansion, stack discovery, natural-language auto-linking, and historical message backfill are excluded.
Implementing or publishing a Bitbucket, Forgejo, or other production plugin is outside this host package. Compatible plugins must adopt the documented registration contract.
Task-creation and passthrough editors retain their current mention-picker scope. Their plain-text PR work uses agent registration when the tools are available.
Delegated task cards, child-result delivery, checkpoints, autonomous PR monitoring, and automatic merge policy are separate work.

## Implementation package

- [System design](../system-design/task-change-registration.md)
- [Plan and work orders](../../../plans/pr-mention-linking/plan.md)
