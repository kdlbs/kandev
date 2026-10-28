---
created: 2026-10-09
status: in_progress
requirements:
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-001
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-002
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-003
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-004
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-005
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-006
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-007
system_design:
  - ../../specs/integrations/system-design/task-change-registration.md
  - ../../specs/integrations/system-design/task-change-registration-plugins.md
legacy_specs: []
---

# Implementation plan: PR mention linking and agent registration

## Overview

Deliver reliable task association through the existing provider stores and neutral tools.
First deliver observation-only agent registration, then selected direct-message references, queue admission and editing, and compatible plugin providers.
Each work order includes its persistence, recovery, UI where relevant, and targeted validation. Execute them sequentially in dependency order.

The complete requirements are [here](../../specs/integrations/requirements/task-change-registration.md); the system design is [here](../../specs/integrations/system-design/task-change-registration.md).
[Investigation](investigation.md) records current code, pinned t3code evidence, contract ownership, and decisions.
The [validation matrix](validation-matrix.md) maps every criterion to planned evidence.

## Scope

### In scope

- GitHub/GitLab and compatible plugin registration with exact host, connection, repository, and change identity.
- Backward-compatible URL/default-task inputs and shared agent guidance.
- Selected-reference intent, admission receipts, provenance, retries, and unlink ordering.
- Existing task list/count updates, direct and queued message feedback, desktop and phone controls.
- Observation-only comparison and automation behavior, including subsequent sync.

### Out of scope

Built-in Azure neutral-tool expansion, production plugin implementation/publication, repository auto-attachment, new polling services, automatic stack discovery, and new automation authority.
Delegation cards, child-result delivery, changed-file checkpoints, commits, pushes, deployment, and production implementation during this design turn.

## Technical approach

Task 01 extends `task_change_link_coordinator.go`, provider stores, MCP DTOs/schema and `sysprompt`.
Its atomic association/intent write seam becomes the shared consumer for later message admissions.
It delivers agent URL registration, GitLab detachment parity, source metadata, revision ordering, and persistent comparison policy.
Its canonical key uses immutable provider repository IDs and explicit connection scope. Paths remain locators, including during legacy repair.

Task 02 extends `EntityReference`, direct-message acceptance, initial-brief paths, event/store projections, and chip/list UI.
It persists intents inside acceptance transactions and resolves them after commit. Its recovery path handles crashes and post-commit event failure.

Task 03 reuses that infrastructure from queue receipt and edit transactions. It carries origin IDs through automatic/manual folds and dispatch.
Queue leases, operation IDs, fingerprint conflicts, incarnation checks, and user-input barriers retain authority.
Task 02 introduces selection IDs through draft and transport metadata. Task 03 uses them to distinguish unchanged edits from remove-and-reselect.

Task 04 bridges plugin backend actions into the same registration flow, tools and UI.
It reuses `MentionSourceBridge`, active provider ownership, `HandleAction`, plugin-owned association storage, and registered review projections.
The [plugin design](../../specs/integrations/system-design/task-change-registration-plugins.md) specifies capability negotiation and lost-reply reconciliation across stores.
Compatible plugin support is included; specific installed Bitbucket/Forgejo versions require adoption of that contract. No production plugin is added to this repository.
Task 04 accepts authorized messages despite association-resolution failure. Null revision fences prohibit restoration during recovery.
Known revision fences never advance on Retry. Lost mutation replies reconcile the original operation receipt before reporting success.

### Implementation decisions

| Boundary | Required delivery | Regression evidence |
| --- | --- | --- |
| Built-in identity | Task/provider/host/scope/immutable repository/change key; verified legacy repair | Rename keeps identity; path reuse cannot inherit a link or tombstone |
| Queue selection | Fresh selection ID on reselection or context-to-link; unchanged edits retain IDs | Identical final text with a fresh selection relinks; unchanged Save and replay do not |
| Plugin admission | Nullable identity/revision, immutable fence, create-if-absent recovery | Resolver outage preserves the message; later unlink prevents recovery from restoring it |

These decisions refine the existing acceptance criteria. The requirements and both designs remain the contract authority.

### Compatibility matrix

| Surface/provider | Identity/transport | Intended result | Verification/fallback |
| --- | --- | --- | --- |
| GitHub mention | Validated reference + WS message/queue | Exact attached target repository PR, observation policy | Direct/queue tests; unresolved repository stays context with reason |
| GitLab mention | Configured host/project/IID + same transport | Same behavior with MR wording and tombstones | GitLab store/service and rendered tests |
| Built-in Azure mention | Existing reference registry | Context only, explicit unsupported state | Capability test; no accidental call to neutral management |
| Compatible plugin mention/URL | Declared backend registration + optional reference search | Same linking UI/tools; plugin owns association | Bitbucket Cloud/Data Center and Forgejo fixtures; exact host/scope/ID tests |
| Repository-only/search-only/older plugin | Partial capability | Discovery/context retained; linking unavailable | No optimistic capability claim or legacy-action bypass |
| Disabled/upgraded plugin | Generation-bound dispatch | Unavailable/uncertain, accepted message retained | Reconciliation after compatible restart; no replay into replacement installation |
| Agent URL | Existing MCP management, both protocol eras | Current-task default, URL identity and safe conflict rejection | Server/backend schema tests and shared guidance tests |
| Legacy agent tuple | Existing MCP fields | Existing explicit target/replacement behavior | Existing management tests remain passing |
| Agent explicit tuple + observation mode | Existing MCP plus additive mode | Registration without comparison changes | Coordinator and downstream event tests |
| Restricted agent mode | Effective catalog/guidance | No unavailable tool promise | Mode matrix in sysprompt/server tests |
| Old client/reference | Intent omitted | Context only, existing Markdown fallback | DTO/draft round-trip tests; no historical backfill |

## ASCII UI preview

These drawings define control order and state placement. Spacing, icons and exact copy are illustrative; all copy uses localization keys.
Desktop controls retain current compact density. Phone targets are at least 44 pixels.

### UI-01: Task chat composer, selected reference, desktop

```text
Fix CI on [repo-a #123 | Link on send v]
          +---------------------------+
          | (o) Link on send           |
          | ( ) Context only           |
          +---------------------------+
                                      [Send]
```

The picker still inserts a chip without sending. The mode label remains visible after the menu closes.
Keyboard opens the chip control, selects a mode, and returns focus to the composer.
Maps to AC-...-001.1, 001.3, 001.6 and 006.4.

### UI-02: Submitted references and task list, desktop

```text
Task                                         [PRs 2 v]
Fix CI on [repo-a #123]  Linked PR #123
          [repo-b #124]  Linking...
          [repo-c #125]  Not linked [Retry] [Details]
Background [repo-a #126] Context only
          [repo-a #127]  Unlinked [Link again]

Linked contributions
  repo-a #123  Open   Selected mention [Original message] [...]
  repo-b #124  Open   Agent registration [Session]        [...]
```

Counts come from persisted provider rows. The example shows independent reference outcomes, not one all-or-nothing batch.
A reference can retain its original label after unlink, but its current status becomes Unlinked.
The existing overflow menu retains Open and Unlink. Unsupported Details explains the scope and offers Open reference.
Maps to AC-...-002.3, 004.4, 006.1 through 006.4.

### UI-03: Phone composer and reference drawer

```text
Fix CI on
[repo-a #123 | Link on send v]
                              [Send]

+--------------------------------+
| repo-a #123                    |
| (o) Link on send               |
| ( ) Context only               |
|                         [Done] |
+--------------------------------+

Submitted message
[repo-a #123]
Linked PR #123
[repo-c #125]
Not linked   [Retry] [Details]
```

The drawer scrolls internally; Done stays reachable above the safe area. The message region remains the page's existing scroll owner.
Long repository names wrap/truncate with accessible full names. No hover-only action is required.
Maps to AC-...-006.4; desktop-to-phone transitions preserve the draft choice.

### UI-04: Queue editor and accepted-link disclosure

```text
Queued prompt 1                                  [Edit] [Remove]
[repo-a #123] Linked PR #123

Edit queued prompt
Fix CI on [repo-b #124 | Link on save v]
Links already added stay linked until you unlink them.
                                            [Cancel] [Save]
```

Phone uses the existing queue edit surface, wraps the reference region, and keeps Cancel/Save touch-accessible.
While the lease is live, editing does not cause association or dispatch. Save links only new intent; Cancel preserves prior accepted state.
After unlink, removing and reselecting a reference shows Link again on save. An unchanged reference stays Unlinked after Save.
The phone drawer exposes the same choice. Draft persistence and breakpoint changes preserve the selection ID.
Maps to AC-...-001.4, 001.5, 005.3 and 006.4.

### UI-05: Origin detail, desktop/phone

```text
repo-a #123
Linked from a selected mention
Original message unavailable
[Open PR] [Unlink]
```

The desktop detail uses current popover/menu primitives; the phone uses the same reference drawer pattern as UI-03.
An unavailable origin is text, never a broken navigation target. Maps to AC-...-004.4 and 006.4.

### UI-06: Plugin autocomplete and capability feedback

```text
Fix CI on #auth
+--------------------------------------+
| Bitbucket Cloud                      |
| team/app #123  Fix authentication     |
| Forgejo (forge.example)               |
| team/app #123  Fix authentication     |
+--------------------------------------+
Selected: [Forgejo #123 | Link on send v]
Legacy:   [Other PR #5 | Context only]
          Task linking unavailable [Details]
Sent:     [Forgejo #123] Linked PR #123
Unresolved: [Forgejo #124] Not linked [Retry]
Lost reply: [Forgejo #125] Status unknown [Retry]
```

This is the same autocomplete, not a separate plugin picker or right-click menu.
Phone uses UI-03's touch drawer; provider/connection details wrap and remain accessible.
A disabled plugin shows Unavailable with last-known state; no false zero count or successful unlink.
Retry preserves the message and original fence. A suppressed result offers Link again for a fresh authorized restoration attempt.
Maps to AC-INTEGRATIONS-CHANGE-REGISTRATION-007.1, .2, .6, .7 and .8.

## Tests

The [matrix](validation-matrix.md) names proposed test files and cases for every AC.
New tests belong to the later implementation turn. Existing targeted tests supply regression anchors, not evidence that this proposal already works.

Run work-order blocks from the repository root. They use isolated subshells to prevent directory drift.
Install web dependencies once before the first frontend command: `(cd apps && pnpm install --frozen-lockfile)`.
Go package tests are intentionally scoped; the repository Make test target is broader than this feature.
No all-worker overrides or overlapping full browser suites are authorized.

## E2E tests

Use new `apps/web/e2e/tests/chat/pr-mention-linking.spec.ts` with project `chromium`.
Use new `apps/web/e2e/tests/chat/mobile-pr-mention-linking.spec.ts` with project `mobile-chrome`.
Task 02 adds direct, initial, unsupported, retry, reconnect, provenance, and unlink/relink flows.
Task 03 adds queue insertion, fold, edit-save/cancel, clear, Send now, and lease/barrier flows to these files.
Use the mock provider/backend fixtures and causal WS/HTTP waits from the existing entity-reference composer E2E tests.
Assert persisted provider rows and unchanged comparison/automation state as well as visible copy.
No live PR creation or real provider mutation is needed.
Task 04 adds `e2e/tests/plugins/pr-registration.spec.ts` and `mobile-pr-registration.spec.ts`, using an installed conformance fixture.
They exercise the same initial/direct/queue, context-only, retry/relink, count, provenance and accessibility flows for plugin providers.
A fixture validates the host contract; it does not certify an uninspected production plugin release.

## Work orders

- [x] [Task 01: Observation-only agent registration](task-01-agent-registration.md)
- [x] [Task 02: Selected direct-message references](task-02-direct-mentions.md)
- [x] [Task 03: Queued references and edit admission](task-03-queued-mentions.md)
- [ ] [Task 04: Plugin registration and conformance](task-04-plugin-registration.md)

Dependency order: **01 -> 02 -> 03 -> 04**. Tasks 01, 02 and 03 are done; Task 04 remains pending.
The shared provider schema and admission model require sequential integration; this plan does not authorize delegation.

## Verification results

Design validation is recorded in [validation evidence](validation-evidence.md).
Tasks 01, 02 and 03 implementation and their targeted backend, PostgreSQL, frontend, browser, and docs checks are complete. Plugin conformance and the remaining package checks are pending with Task 04.
The shared requirement and system-design documents remain `draft` until Task 04 completes. The plan remains `in_progress` while that work order is open.

## Risks

- GitLab soft detachment touches active-list, watch, automation, and deletion queries. Missing one consumer can restore a link.
- Legacy repository identities can be incomplete or collide. Migration must preserve uncertainty and exact cleanup.
- Comparison calls also occur on later sync and re-detection. Tests must inspect runtime side effects, not just final row values.
- Accepted queue removal preserves links by design. UI disclosure must make this outcome clear.
- Two SQL dialects and acceptance/write crash recovery require transaction tests with controlled interleavings.
- Plugin writes cross a process/storage boundary. A timeout can follow a committed association; operation receipts and readback must preserve uncertainty.
- Plugins must adopt the versioned backend contract; repository/search support alone is insufficient.
- Built-in repositories without verified immutable IDs require explicit repair. Startup migration must not resolve them through a potentially reused path.
- Plugin resolution and host admission cannot share a transaction. A stale fence can require another Link again action even after message acceptance.
- Updated guidance increases compliance but cannot guarantee that every external agent follows instructions. Persisted tool results remain the source of truth.

There are no material unanswered product questions. Changes to these decisions require a design update during later implementation.
