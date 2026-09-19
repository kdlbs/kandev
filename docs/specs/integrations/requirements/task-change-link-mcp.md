---
status: active
system: integrations
created: 2026-09-14
updated: 2026-09-14
owners:
  - kandev
---

# Manage Task Pull Requests and Merge Requests Requirements

## Overview

Agents coordinating task delivery need to attach, swap, or remove the external
change request (a GitHub pull request or a GitLab merge request) that carries
the task's delivery after a task already exists. The MCP surface exposes provider-neutral
operations so an agent can state intent without learning
provider-specific store mechanics, and every mutation answers with the
resulting canonical linked set so the caller can verify immediately.

This requirement is owned by the integration system because the contract
spans both provider stacks and one shared task-facing tool surface, while the
task system owns the task identity and the workspace system owns repository
attachment.

## Terminology

- **Change request (CR):** A GitHub pull request or a GitLab merge request.
- **Canonical repository identity:** The persisted repository identity
  (`repository_id`, resolved through the task's workspace) that distinguishes
  a fork from the canonical repository even when both contain the same
  change-request number.
- **Active association:** The stored link between the task and one
  provider-specific change request; historical receipts (terminal transcripts,
  conversation history) are unaffected by unlinking.

## Design status

This requirement extends the association contract merged in PR #3506. The
existing association guarantees remain required. The four-tool contract is
implemented in the 0.95.0 release transition. The integration system owns
contribution identity, automation, and provider capabilities.

## Requirements

### REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001: Manage task change requests through MCP

**Intent:** Give agents safe, verifiable control over the external delivery
record of an existing task.

**User story:** As an agent coordinating a task, I want to link, unlink, or
replace a GitHub pull request or GitLab merge request on that task through
MCP, so that the task's delivery trail points at the real change request
without me editing task records by hand.

#### Acceptance criteria

- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1:** When a mutation request is
  authorized for the calling task's workspace and supplies the task ID,
  provider (`github` or `gitlab`), canonical repository identity, and the
  change-request number, the system applies the link, unlink, or replacement
  and returns the resulting canonical linked set in the same response.
  A bare number without repository identity is rejected, and same-number
  change requests in a fork and a canonical repository remain distinct
  associations.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.2:** When a caller requests an
  operation for a task outside its reachable workspace set, the system
  rejects the request without mutating any store.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.3:** When a link targets an
  already-active association, the system succeeds without duplicating it;
  when an unlink targets an absent association, the system succeeds.
  Unlink removes only automation and settings scoped to that exact
  provider, repository, and number, and leaves other linked change requests
  and task metadata untouched.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.4:** When a replacement's new
  target cannot be established, the previous association remains active; when
  the established new association also cannot be removed during rollback, the
  response reports both failures so the caller can reconcile the partial
  state.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.5:** When a repository cannot be
  resolved to a verified provider host (including a persisted identity with
  no provider host), link operations fail closed instead of assuming a
  default provider host.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.6:** When a mutation succeeds,
  query surfaces and connected clients observe the new linked state after a
  restart, and unlinking changes only the active association, never
  conversation or terminal-receipt history.

### REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002: Discover change requests and capabilities

- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002.1:** `get_task_change_requests_kandev` shall return the current task's linked contributions, status, automation settings, and provider capabilities.
  Each contribution shall include `provider`, canonical `repository_id`, and positive integer `number`.
  Missing or ambiguous historical identity shall be reported explicitly, without guessing a repository.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002.2:** The task catalog shall advertise one shared management, read, and automation tool set for supported providers.
  GitHub-capable tasks shall also expose the outcome-reporting tool. GitLab-only tasks shall not expose that tool.
  Unsupported providers shall not gain capabilities through tool visibility. Existing mode restrictions shall remain effective.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002.3:** `list_tasks_kandev` and `list_related_tasks_kandev` shall retain `change_requests` and the legacy GitHub-only `prs` field.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002.4:** Failed provider reads shall identify incomplete results. They shall not appear as an empty linked set or disabled settings.
  Capability support shall remain distinct from temporary credential or provider availability.

### REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003: Manage associations with explicit intent

- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.1:** `manage_task_change_request_kandev` shall require `operation: link | unlink | replace`, target task, and complete contribution identity.
  Replacement shall additionally require complete old identity. Other operations shall reject old identity fields.
  Unknown fields, unsupported providers, fractional numbers, and incomplete identities shall fail before mutation.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.2:** Replacement shall remain a single call and permit only the same provider.
  It shall establish the new link before removing the old link. An identical replacement shall be a no-op.
  Compensation shall never remove a new link that already existed before the request.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.3:** Stale associations shall remain removable after repository detachment.
  Removal events shall describe committed deletion only. Failed replacement shall report original failure, compensation failure, and known remaining associations.
  When remaining state cannot be read, the response shall identify that uncertainty.

### REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004: Target automation explicitly

- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.1:** `update_task_change_request_automation_kandev` shall require an explicit association target or task target with a nonempty provider selection.
  Association targets shall require complete canonical identity. Task targets shall reject association identity.
  Read and automation operations shall remain bound to the current task. Supplied execution identity shall be rejected.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.2:** Association updates shall change only that association's auto-fix, auto-merge, or lifecycle notification switches.
  Task updates shall change those switches for currently linked contributions of explicitly selected providers, including both providers on mixed tasks.
  They shall not establish defaults for future links. An empty switch target shall be rejected.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.3:** Custom auto-fix prompts shall remain task-level and provider-scoped.
  Only task targets shall accept `auto_fix_prompt_override`. Selecting both providers shall update each provider's prompt separately.
  An empty string shall clear the override. Prompt-only updates shall work without linked contributions.
  Omitted fields shall remain unchanged. Null fields, empty patches, and lifecycle prompt overrides shall be rejected.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.4:** Unsupported fields or unresolved targets shall fail preflight without writes.
  Cross-provider runtime failures shall report applied, failed, and unattempted providers, plus known resulting state.
  The response shall not claim cross-provider atomicity or encourage retry of already-applied updates.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.5:** Trusted principal checks, task workspace authorization, workspace credentials, and provider-origin checks shall apply to every call.
  GitLab project paths shall be resolved internally only from verified association or repository identity.
  Ambiguous identity shall fail closed without affecting another contribution.

### REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005: Report a bound auto-fix outcome

- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005.1:** `report_change_request_auto_fix_outcome_kandev` shall accept only `outcome` and a nonempty `summary`.
  Outcomes shall remain `action_taken`, `non_actionable`, and `blocked`. Task, session, turn, provider, and contribution identity shall remain server-owned.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005.2:** Reports shall preserve the existing GitHub attempt protocol, first-write behavior, replay rules, and provider-progress requirements.
  Ordinary turns, stale turns, foreign sessions, and conflicting reports shall not mutate attempts or enable automation.
  Unmatched reports shall explain that ordinary work must finish without retrying the report or enabling auto-fix.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005.3:** GitLab shall report outcome-protocol support as false until its own bound implementation exists.
  A mixed task's GitLab turn shall not report against a GitHub attempt because the tool is visible.

### REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-006: Migrate callers and discovery

- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-006.1:** The released catalog shall remove all eight superseded tool names.
  New injected instructions, built-in workflows, skills, and public examples shall use the shared contract.
- **AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-006.2:** The migration shall document cached catalogs, old runtime binaries, queued prompts, and user-edited callers.
  It shall preserve user-edited prompts and historical transcripts. Any temporary transport compatibility shall have a release-bound retirement rule.
  Both supported MCP protocol eras shall expose the same contract and validation behavior.

## Exclusions

Provider stores, REST/WebSocket UI settings, rendered UI, provider automation algorithms,
and cross-provider replacement are unchanged. GitLab outcome tracking is excluded.
No new provider, database migration, or generic provider persistence framework is required.

## Implementation Plans

- [Provider-neutral change request tools](../../../plans/provider-neutral-change-request-mcp/plan.md): the implementation package.
- [Merged association delivery](../../../plans/manage-task-pr-and-m/plan.md): historical baseline, completed in PR #3506.
