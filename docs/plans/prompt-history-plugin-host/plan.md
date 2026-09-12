---
created: 2026-09-06
status: done
spec: "../../specs/plugins/requirements/prompt-history-extraction-host.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-001
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-003
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-004
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
system_design:
  - "../../specs/plugins/system-design/prompt-history-extraction-host.md"
legacy_specs: []
---

# Implementation Plan: Prompt History Plugin Host Prerequisites

## Objective

Land the complete public browser Host boundary required by a future external prompt-history plugin while keeping the shipped core panel unchanged. The package adds capability-gated session reads, typed live reconciliation, scoped native navigation, host-owned prompt display dependencies, and the missing generic task-panel context.

This plan does not implement or publish the external plugin and does not remove or migrate the core panel.

## Contract decisions

- Browser plugins consume `host.conversation` from `@kandev/plugin-sdk`; they do not import `apps/web`, inspect `host.store`, call `/api/v1` directly, or register raw session message handlers.
- Authenticated `/api/plugins/{id}/conversation/...` routes reuse task services, enforce `api_read:messages`, and return narrow sanitized DTOs. Existing first-party REST and Go Host RPCs stay compatible.
- Conversation hooks are resolved through a Host-created panel scope carrying panel/task/session/generation; IDs that differ from the active scope are rejected. They own snapshot/subscription ordering, pagination, reconnect reconciliation, and plugin-generation cancellation. The future plugin owns prompt filtering/derivation presentation only through returned DTOs.
- Task-panel registration grows optional localized-title and visibility contracts. Task-panel props grow session kind and a generation-bound message-navigation capability.
- Custom-prompt alias rendering and favorite state remain host-owned because they depend on private settings and browser stores. The API is narrow: one curated renderer and one read-only hook.
- Core prompt history remains present until a later package proves the external plugin, migrates saved panel IDs, and removes core ownership.

## Work packages

### Wave 1: Contract

- [Task 01: Publish browser conversation contract](task-01-publish-browser-conversation-contract.md)

### Wave 2: Independent host foundations

- [Task 02: Add plugin conversation read routes](task-02-add-plugin-conversation-reads.md) (depends on Task 01)
- [Task 03: Extend generic task panel capabilities](task-03-extend-task-panel-capabilities.md) (depends on Task 01)

### Wave 3: Frontend facade

- [Task 04: Build browser conversation Host facade](task-04-build-conversation-host-facade.md) (depends on Tasks 01-03)

### Wave 4: Parity proof

- [Task 05: Prove prompt-history plugin parity](task-05-prove-plugin-parity.md) (depends on Task 04)

### Wave 5: Documentation and package verification

- [Task 06: Document and verify Host prerequisites](task-06-document-and-verify.md) (depends on Tasks 02-05)

### Acceptance ownership

Each acceptance criterion has one primary work-order owner. Where a criterion
contains separate transport and facade sub-criteria, the plan records explicit
sub-criterion ownership; work orders may verify a behavior owned elsewhere but
must not silently assume it.

| Criteria | Primary owner |
| --- | --- |
| AC-002.1 public shapes and error mapping | Task 01 |
| AC-002.1 scope revision, CAS, and atomic page lifecycle | Task 04 |
| AC-002.2-3 | Task 01 |
| AC-001.1-7, AC-003.1-4 | Task 03 |
| AC-004.1-3 | Task 04 |
| AC-002.4, AC-002.7-8, AC-002.10, AC-002.13 | Task 02 |
| AC-002.15-16 transport, persistence, and compatibility adapter | Task 02 |
| AC-002.11, AC-005.2-3 | Task 05 |
| AC-005.1 | Task 01 |
| AC-005.4 | Task 06 |

Tasks 02 and 03 own disjoint backend and frontend trees and are parallel-safe. Execution remains sequential unless the user explicitly authorizes subagents.

## Mobile design contract

- Desktop entry remains the task workbench's add-panel menu and dockview panel.
- Mobile entry remains the grouped Panels bottom-navigation action and existing inset picker; the panel renders as the existing full-height plugin task panel.
- The prompt list owns one vertical scroll area. Row navigation is the primary tap; expansion is a distinct 44 px touch action. Alias preview uses the existing coarse-pointer drawer.
- Shared hooks, pagination, derived prompt entries, and navigation outcomes are identical across presentations. Mobile owns only the picker, full-height composition, touch geometry, and local chat scroll target.
- Pixel 5 Playwright must open the fixture plugin panel, traverse older pages, open an alias preview, navigate to a prompt in Chat, and assert no document horizontal overflow.

## TDD and verification strategy

Every implementation work order follows RED-GREEN-REFACTOR: add the focused behavioral failure first, implement the smallest contract, then refactor without broadening scope.

Verification is staged rather than deferred to a single repository-wide run:

1. contract assignability and SDK typecheck;
2. focused backend route/service tests;
3. focused frontend Host, hook, task-panel, registry, and mobile component tests;
4. desktop and mobile fixture-plugin Playwright after fresh web/backend builds;
5. i18n, public-doc, frontend typecheck/lint, backend lint/test, and affected build commands.

The core prompt-history unit and E2E suites remain part of the final regression proof because this package must not change shipped ownership or behavior.

## Risks and controls

- **Snapshot/event gap:** subscribe and await readiness before the first snapshot; buffer and merge by stable ID and `updatedAt`.
- **Stale session or plugin generation:** generation-key every request, subscription, and navigation capability; abort and revoke on identity/lifecycle change.
- **System prompt leakage:** map a dedicated DTO and test stripped content plus omitted `raw_content`/metadata.
- **Capability bypass inside supported APIs:** gate plugin-scoped routes server-side. Document that same-origin native plugins are not a hard sandbox.
- **Mobile navigation ownership:** inject the existing local mobile adapter; do not globalize `mobileScrollTarget`.
- **Public/internal type drift:** update `@kandev/plugin-sdk`, `PLUGIN-API.md`, internal aliases, host implementation, and assignability tests in the same contract package.
- **Accidental extraction:** keep core registrations, layout ID, stores, locale keys, and tests untouched; fixture contributions use a distinct plugin ID.

## Completion boundary

This package is complete when all six work orders are done and a fixture plugin proves the public boundary on desktop and mobile. External plugin creation, publication, core removal, and saved-layout migration remain blocked on a later explicitly approved design and implementation package.

## Current result

Delivered the additive SDK, authenticated browser reads, task-panel context and
navigation, browser facade, host-owned display adapters, ordered transport,
durable primary-database journal/versioning, atomic poison rebind, lifecycle
fencing, and desktop/mobile fixture proof while preserving core Prompt History
ownership. Retention now runs from the live service maintenance lifecycle.

All six work orders are complete. External plugin creation, publication, core
removal, and saved-layout migration remain blocked on a later explicitly
approved design and implementation package.
