---
created: 2026-09-08
status: complete
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-001
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-002
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-003
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-004
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-005
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-006
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-007
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-008
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-009
system_design:
  - ../../specs/platform/system-design/agent-settings-parity.md
  - ../../specs/platform/system-design/agent-settings-domains.md
legacy_specs: []
---

# Implementation plan: Agent-accessible Kandev settings

## Overview

Deliver agent access to the core Kandev settings domains, not only profile settings.
Four compact settings tools provide search, descriptions, reads, and updates.
One bounded resource-lookup tool supplies authorized targets. Explicit lifecycle tools remain separate.

Platform owns the cross-domain interface and coverage contract. Existing domain services own data, validation, permissions, and persistence.
The existing paths and requirement IDs remain stable after this scope expansion.
The implementation was completed in the primary conversation after the explicit implementation request.
The work-order evidence below records the delivered scope and its validation limits.

This design package replaces the earlier profile-only delivery scope.
Implementation evidence is recorded below, including the focused checks and the unrelated full-suite failures.

## Scope

### In scope

- All eligible fields in requirements 002 and 006, with independent field/control coverage.
- Concrete and dynamic profiles, separate profile MCP documents, and editable agent definitions.
- Portable preferences, keybindings, task behavior, saved views, utility defaults, and LSP preferences.
- Workflows, steps, workspaces, repositories, scripts, repository sets, executors, profiles, and environments.
- Permitted task settings, saved prompts, utility agents, editor definitions, and notifications.
- Noncredential built-in integration defaults, scopes, presets, and watch configuration.
- Existing workspace automation definitions and triggers.
- Permitted runtime overrides and storage-maintenance policies.
- Authorized target lookup, shared schemas, dynamic choices, safe errors, and source provenance.
- Domain notifications, live UI reconciliation, dirty-draft recovery, and desktop/mobile evidence.
- Existing lifecycle compatibility, plus explicit prompt/utility/editor/notification creation where absent.
- CI completeness, schema freshness, search quality, stable tool envelopes, prompt guidance, and public docs.

### Out of scope

- New authoritative settings files, file import/export, or a central settings database.
- Browser/device-local persistence changes or a remote browser bridge.
- Deployment-owned startup writes, arbitrary environment mutation, or database relocation.
- Credential enrollment, secret retrieval, account authority, authentication enablement, or MCP permission expansion.
- Plugin-owned settings APIs and Office's separate organization-management model.
- New runtime flag identities, shipped-default changes, automatic restarts, and immediate cleanup.
- Installation, login, arbitrary execution, and general lifecycle redesign.
- Universal optimistic locking or atomic transactions across independent resources.

## Technical approach

### Inventory before registration

Task 00 inventories routes, UI controls, mutation payloads, nested contracts, and exceptions.
It records field-level evidence and maps eligible entries to the domain work orders.
The navigation catalog is one input, not proof of complete field coverage.

Task 01 introduces shared catalog types and profile contracts.
The exporter produces `contract.generated.json` plus the profile subset under `apps/web/lib/settings-discovery/`.
Independent Go contract roots and frontend payload types detect omissions.
The final delivery gate rejects pending eligible entries, and the completed catalog contains no pending eligible field.

### Stable tools and targets

Task 02 implements `update_settings_kandev({target, changes, options?})`.
Task 03 adds search, description, saved-value reads, and `list_settings_resources_kandev`.
The common target is `{resource_type, resource_id?, workspace_id?}`.
Personal and installation singletons reject arbitrary resource IDs. Entity adapters resolve and authorize ownership.

Descriptions return field, context, and operation-option schemas on demand.
The outer tool schemas contain no growing field enums or resource-schema unions.
Existing external tools stay compatible. Each configuration alias is removed only after its replacement passes parity checks.

### Domain-owned execution

Every adapter calls an existing domain controller or service with trusted identity.
A schema validates shape. The domain validates resulting state, relationships, and mutation effects.
An unlocked read/merge/full-write adapter is not acceptable when it can erase another writer's changes.
Required atomic patches belong in the shared domain path, with rollback and competing-writer evidence.

The task DTOs contain untagged internal request fields. Shared tagged wire contracts must preserve actual HTTP names.
Opaque maps need typed domain schemas or explicit exclusions.
Adapters preserve omitted credentials and never reconstruct a replacement document from a redacted read.

Runtime overrides retain registry metadata, environment locks, explicit null reset, and pending-restart state.
Storage settings retain complete-document replacement and exact acknowledgement requirements.
Neither adapter restarts Kandev or performs immediate maintenance.

### Visible results

Task 04 establishes profile draft reconciliation through the current save coordinator.
Every later domain work order includes its store event path, editor state, and visible result.
Clean editors adopt current values. Dirty drafts require baseline recovery without losing local edits.
Own-save acknowledgements and late responses cannot erase newer edits.

Existing domain events remain authoritative. Missing events use scoped value-free invalidation and guarded refetch.
User settings reuse their shared wire mapper and revisions. Workflow and executor contributors retain domain ownership.
Phone and desktop use existing routes and save controls. Recovery remains inline, without new overlays.

### Delivery completeness

Task 19 closes the required-domain matrix and validates the end-to-end configuration workflow.
Its inventory checks reject pending eligible fields and unclassified controls.
It is a focused feature acceptance gate, not a generic QA, review, or full-suite task.
Public docs describe actual coverage and explicit exceptions, without an unmeasured coverage percentage.

## Tests

The evidence below records the implemented checks. Shared adapter tests cover the domain matrix,
while the browser scenarios cover the real MCP transport and desktop/mobile user path.
Numeric criteria use the prefix `AC-PLATFORM-AGENT-SETTINGS-PARITY-`.

| Acceptance criteria | Evidence |
| --- | --- |
| 001.1 through 001.8, 005.1, 005.3, 006.11, 009.1 | `apps/web/lib/settings-discovery/coverage-inventory.test.ts`, `delivery-coverage.test.ts`, and `apps/backend/internal/settingscatalog/*_test.go` |
| 002.1 through 002.6 | `apps/backend/internal/agent/settings/dto/profile_contract_test.go`, controller contract tests, generated profile contract tests, and the registry field check |
| 002.7 through 002.9, 005.2, 005.5 | `apps/backend/internal/mcp/handlers/settings_handlers_test.go`, `settings_tools_test.go`, and `apps/backend/internal/backendapp/settings_operations_test.go` |
| 003.1 through 003.5 | `settings_operations_test.go` authorization and target checks, plus the compact-tool transport tests |
| 005.4 | `scripts/settings-contract-ci.test.mjs`, exporter `--check`, and backend/frontend workflow guards |
| 007.1 through 007.5 | Resource lookup tests in `settings_handlers_test.go` and domain adapter tests in `settings_operations_test.go` |
| 004.1 through 004.5 | `apps/web/components/settings/agent-profile-reconciliation.test.ts`, profile MCP reconciliation code, and the desktop/mobile scenarios |
| 006.1 through 006.10 | The domain registry and shared adapter tests cover user, workflow, workspace, execution, task, prompt, utility, editor, notification, integration, automation, runtime, and storage descriptors |
| 008.1 through 008.6 | Adapter validation, scope, redaction, sensitive-field, lifecycle-exception, and integration tests in `internal/backendapp` and `internal/mcp` |
| 009.2, 009.3, 009.4, 009.5 | `delivery-coverage.test.ts`, generated snapshot checks, public-doc coverage validation, and the real MCP desktop/mobile workflow |

Each work order names the exact files and commands.
For changed SQL queries, domain tests cover supported SQLite and PostgreSQL behavior.
No database migration is planned. A required migration triggers the repository persistence-conformance requirements before implementation proceeds.

## E2E tests

Task 19 owns `settings-agent-configuration.spec.ts` and its `mobile-` companion for the complete user flow.

All scenarios use real registered MCP transport, isolated domain fixtures, causal waits, visible controls, and reload persistence.
They do not dispatch raw `mcp.*` gateway actions or use real external credentials.
Desktop uses `chromium`. Phone uses `mobile-chrome` with the configured device and touch controls.
The managed runner builds production assets. Runs remain sequential and respect the repository worker budget.
The implemented delivery scenario searches, describes, reads, updates, and resolves a workspace resource through the registered MCP transport.
Per-domain validation remains in the backend adapter and catalog suites rather than duplicating browser scenarios for each domain.

## Work orders

- [x] [Task 00: Establish the complete settings inventory](task-00-coverage-inventory.md)
- [x] [Task 01: Establish the profile settings contract](task-01-profile-contract.md)
- [x] [Task 02: Deliver profile mutation parity](task-02-profile-mutations.md)
- [x] [Task 03: Expose settings discovery](task-03-settings-discovery.md)
- [x] [Task 04: Reconcile live profile edits](task-04-editor-reconciliation.md)
- [x] [Task 05: Expose portable user settings](task-05-user-preferences.md)
- [x] [Task 06: Expose workflow settings](task-06-workflow-settings.md)
- [x] [Task 07: Expose workspace resource settings](task-07-workspace-settings.md)
- [x] [Task 08: Expose execution resource settings](task-08-executor-settings.md)
- [x] [Task 09: Expose task configuration settings](task-09-task-settings.md)
- [x] [Task 10: Expose saved prompt settings](task-10-prompt-settings.md)
- [x] [Task 11: Expose utility agent settings](task-11-utility-settings.md)
- [x] [Task 12: Expose editor definition settings](task-12-editor-settings.md)
- [x] [Task 13: Expose notification provider settings](task-13-notification-settings.md)
- [x] [Task 14: Expose issue integration settings](task-14-issue-integration-settings.md)
- [x] [Task 15: Expose code-host integration settings](task-15-codehost-settings.md)
- [x] [Task 16: Expose workspace automation settings](task-16-automation-settings.md)
- [x] [Task 17: Expose permitted runtime overrides](task-17-runtime-settings.md)
- [x] [Task 18: Expose storage maintenance policies](task-18-storage-settings.md)
- [x] [Task 19: Complete core settings coverage](task-19-delivery-coverage.md)

Dependency order: 00 through 19, sequential.
Profile work establishes the shared path. Core domain adoption follows. Delivery checks close the complete inventory.
All work orders are complete; the individual result sections record the shared-adapter implementation where several domains use one domain-owned operation path.

## Verification results

Implementation checks passed on 2026-09-09:

- `rtk go test ./internal/backendapp ./internal/mcp/handlers ./internal/mcp/server ./internal/settingscatalog ./internal/agent/settings/...`: 2,338 tests passed in 12 packages.
- `rtk go run ./cmd/settings-catalog --check`: passed.
- `rtk node --test scripts/settings-contract-ci.test.mjs`: 3 tests passed.
- `rtk node --test scripts/validate-public-docs.test.mjs`: 61 tests passed.
- `rtk pnpm exec vitest run lib/settings-discovery/*.test.ts components/settings/agent-profile-reconciliation.test.ts lib/ws/handlers/agents-settings-updated.test.ts`: 46 tests passed in 9 files.
- `rtk pnpm run typecheck`, `rtk pnpm run lint`, and `rtk pnpm run i18n:check`: passed.
- Desktop and mobile real-MCP settings scenarios: 1 test passed in each project.
- `rtk python3 scripts/lint-spec-files.test.py`: 30 tests passed.
- `rtk python3 scripts/lint-spec-files.py --all`: passed.
- `rtk git diff --check`: passed.

The full `rtk make -C apps/backend test` suite was also attempted. It remains red in unrelated baseline or environment-sensitive probe, config-discovery, launcher, and Office SQLite tests; all settings-related packages passed.

## Risks

- Generated schemas can misrepresent null, omission, and opaque nested maps.
- HTTP middleware does not protect in-process MCP calls. Every adapter needs trusted-context authorization.
- Full-document writes can erase unrelated concurrent changes. Shared domain atomicity must protect all writers.
- More settings coverage increases the importance of useful search results and exact resource selection.
- Secrets can appear in provider configuration or free-form content. Metadata and lookup never search those values.
- Events without revisions require invalidation and guarded refetch, not timestamp guesses.
- Integration providers and executor capabilities can be unavailable independently of saved settings.
- Runtime override persistence does not imply an immediate runtime change.
- Broad scope cannot be marked complete after profile parity or a partially populated registry.
- Browser-local, deployment-owned, account-security, and plugin-owned exceptions remain explicit.

## Design references

- [Requirements](../../specs/platform/requirements/agent-settings-parity.md), IDs 001 through 009.
- [Shared interface](../../specs/platform/system-design/agent-settings-parity.md).
- [Required domains](../../specs/platform/system-design/agent-settings-domains.md).
- [Ownership decision](../../decisions/2026-09-08-domain-owned-settings-catalog.md).
