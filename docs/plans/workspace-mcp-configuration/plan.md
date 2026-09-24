---
created: 2026-09-01
status: complete
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-002
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-004
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-007
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
legacy_specs: []
---

# Implementation Plan: Workspace MCP Configuration

## Overview

Replace raw profile MCP JSON with a workspace-owned catalog, typed scope
selections, and one effective-set resolver. Build the storage and API contract
first, then connect Registry discovery, runtime delivery, responsive settings,
and end-to-end evidence.

The order protects workspace and secret boundaries before any UI exposes the
feature. It also lets runtime and frontend tasks depend on one stable definition
and selection contract.

## Scope

### In scope

- Workspace CRUD for custom and installed MCP definitions.
- Curated templates and a cached public MCP Registry aggregator.
- Workspace-contextual selections for profiles plus repository, task, and task
  agent session selections.
- Additive union with stable-ID deduplication and origin reporting.
- ACP and passthrough delivery from the composed effective set.
- Idle-session ACP resume/load reconfiguration with applied-state feedback.
- Idempotent migration from raw profile MCP configuration.
- Desktop, phone, keyboard, localization, E2E, and public documentation.

### Out of scope

- Private registries, ratings, reviews, security scanning, and trust badges.
- Marketplace execution, automatic installation, or automatic updates.
- Subtractive selection, precedence, and per-scope configuration overrides.
- MCP list replacement during an active turn.
- Removal of the legacy storage table in the first release.

## Technical approach

### Workspace catalog and persistence

Extend `apps/backend/internal/agent/mcpconfig` with catalog models, validation,
repository interfaces, selection interfaces, and a single effective resolver.
Keep existing ACP and passthrough conversion functions as delivery adapters.

Add replayable SQLite and Postgres migrations for definitions, registry cache,
typed selection joins, session selection revisions, and legacy import state.
Wire catalog repositories through the existing settings and task dependency
assembly. Extend workspace reset and cleanup fixtures for all new rows.

Add workspace-authorized handlers beside the current agent settings handlers.
Use typed profile, repository, task, and task-session selection routes. Catalog
responses contain sanitized configuration and secret references only.

### Registry aggregation

Add `internal/agent/mcpconfig/registry` with a bounded Registry API client, a
cursor-paginated full sync, `updated_since` incremental sync, hourly scheduling,
single-flight manual refresh, and last-good-cache fallback. Store deprecated
and deleted state. Embed a small versioned curated manifest and merge both
sources in marketplace search.

Marketplace installation validates the cached entry revision and copies the
reviewed package or remote metadata into an independent workspace definition.
It never starts, connects to, or downloads the server.

Remote definitions require no local installation. Managed npm definitions pin
an exact version and materialize lazily through Kandev's Node runtime in each
task executor. The executor cache stays outside the repository. Other Registry
package types remain visible but unavailable until a typed materializer exists.

Custom setup offers remote endpoint, managed npm package, and existing
executable modes. Saving any mode performs no execution. Existing executables
must already exist inside the task executor.

### Scope selection and migration

Replace selection sets atomically for each scope. Profile selection APIs always
include a workspace ID, even for global profiles. Task and session creation
requests gain optional MCP definition IDs.

Add an idempotent compatibility importer for `agent_profile_mcp_configs`.
Create deterministic workspace-local definitions and profile selections for
each profile-workspace pair. Preserve the legacy row as runtime fallback until
that pair imports successfully.

### Effective runtime resolution

Change `manager_profile.go` and every start, restart, resume, reset, and rebind
path to build a typed resolution context. Union all repository, profile, task,
and session selections by stable definition ID. Accumulate origins, sort by
runtime name, resolve secrets at delivery time, then apply provider and executor
transport policy.

Materialize managed npm packages after composition and before delivery. Use the
exact package version and a Kandev-owned executor cache. Resolve custom existing
commands inside the executor and fail that server if its executable is absent.

Extend `MCPAttachmentReport` with definition revisions and bounded origins.
Never serialize the ephemeral resolved-secret delivery value.

### Idle-session application

Add ACP `session/resume` support to the client and adapter while preserving
`session/load`, which already carries an MCP list and emits attachment evidence.
Read both capabilities from the ACP `initialize` response, not from the static
`agents.RuntimeConfig.SupportsSessionResume` registry flag. Session selection
state tracks desired revision, applied revision, application state, and
sanitized failure details.

An active turn records `pending_idle` without provider mutation. An idle
session prefers advertised resume, falls back to advertised load, and records a
new attachment attempt on success. Unsupported paths become
`deferred_restart`. Failed reconnects preserve the prior applied revision.

### Responsive frontend

Add `MCP servers` to `workspace-settings-tabs.ts` and build configured and
marketplace routes under workspace settings. Use a single-column mobile list
and a full-height setup surface for the deep configuration form.

Replace `profile-mcp-config-card.tsx` with the shared selection picker after the
compatibility API is available. Reuse the picker in repository settings, task
creation, new task agent creation, and idle-session settings. On mobile, use
the established bottom-sheet picker with 44-pixel rows and one scroll owner.

Put repository MCP selection in a collapsed-by-default section with a count in
its disclosure header. Put task MCP selection only inside the existing collapsed
Advanced settings section on desktop and mobile.

All copy uses i18next. Add English, Portuguese, Simplified Chinese, and both
Traditional Chinese catalogs. Generate Traditional Chinese with
`pnpm run i18n:zh-hant`.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-...-001.1` through `001.8` | Catalog service, repository, handler, authorization, execution-mode, conflict, disable, impact, and non-execution tests in `internal/agent/mcpconfig` and agent settings handlers |
| `AC-...-002.1` through `002.11` | Fake Registry API tests for pagination, incremental sync, stale cache, package and remote choices, exact versions, compatibility, review installation, and non-execution |
| `AC-...-003.1` through `003.7` | Selection repository and resolver tests for every scope, global-profile workspace context, multi-repository union, origins, and invalid cross-workspace IDs |
| `AC-...-004.1` through `004.8` | Lifecycle and passthrough tests for deterministic delivery, filtering, definition revisions, secret failures, attachment evidence, the executor-exposure boundary, and typed unresolvable-command evidence |
| `AC-...-005.1` through `005.9` | ACP client, adapter, and lifecycle tests for active-turn deferral, resume preference, load fallback, unsupported deferral, failure rollback, identity preservation, and preserved-or-reported wakeup state |
| `AC-...-006.1` through `006.8` | Frontend unit tests, disclosure-state tests, i18n checks, desktop Chromium E2E, and mobile Chrome E2E |
| `AC-...-007.1` through `007.5` | Idempotent migration, per-workspace global profile, fallback, replay, and redaction tests |

Test names or nearby comments use `@covers AC-...` where the file location does
not make the requirement mapping clear.

## E2E tests

- `apps/web/e2e/tests/settings/workspace-mcp-configuration.spec.ts`, Chromium:
  create a custom definition and browse cached marketplace entries. Review
  setup, select scopes, and confirm impact before deletion.
- `apps/web/e2e/tests/settings/mobile-workspace-mcp-configuration.spec.ts`,
  mobile Chrome: search the marketplace and open the full-height setup flow.
  Verify one scroll owner, keyboard labels, touch targets, and no overflow.
- `apps/web/e2e/tests/task/task-mcp-selection.spec.ts`, Chromium: create a task
  with repository, profile, and task selections. Confirm the union and origins,
  then add an idle-session selection and observe applied state.
- `apps/web/e2e/tests/task/mobile-task-mcp-selection.spec.ts`, mobile Chrome:
  use the task and session selection sheets, inspect inherited origins, and
  verify pending, deferred, and failure states without overflow.

These flows cover selected catalog criteria, `002.1` through `002.11`, and
`003.1` through `003.7`. They also cover `005.1` through `005.8` and `006.1`
through `006.8`.

## Work orders

Wave 1:

- [x] [Task 01: Persist the workspace MCP catalog](task-01-persist-workspace-mcp-catalog.md)

Wave 2:

- [x] [Task 02: Integrate the public MCP Registry](task-02-integrate-public-mcp-registry.md)
- [x] [Task 03: Migrate scoped MCP selections](task-03-migrate-scoped-mcp-selections.md)

Wave 3:

- [x] [Task 04: Resolve effective runtime MCPs](task-04-resolve-effective-runtime-mcps.md)
- [x] [Task 06: Build workspace MCP settings](task-06-build-workspace-mcp-settings.md)

Wave 4:

- [x] [Task 05: Apply idle-session MCP changes](task-05-apply-idle-session-mcp-changes.md)

Wave 5:

- [x] [Task 07: Add scoped MCP selectors](task-07-add-scoped-mcp-selectors.md)

Wave 6:

- [x] [Task 08: Cover MCP user journeys](task-08-cover-mcp-user-journeys.md)
- [x] [Task 09: Update public MCP documentation](task-09-update-public-mcp-documentation.md)

Execution is sequential in the primary conversation. Wave labels identify
dependency readiness and do not authorize subagents.

## Verification results

- `env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE make -C apps/backend test` passed for the full Go backend suite.
- `make -C apps/backend build`, `make -C apps/backend e2e-plugin-package`, and `pnpm run build:e2e` passed.
- Web lint and typecheck passed.
- Localization checks, new-code ratchet, E2E sleep-ratchet, public-doc tests, public-doc validation, and specification lint passed.
- Focused changed web tests passed: 80 tests in 5 files.
- MCP E2E suites passed: desktop settings 4 tests, mobile settings 2 tests, desktop task 1 test, and mobile task 2 tests.
- The repository-wide Vitest command was attempted with the default and single-worker settings. Its worker exited unexpectedly after emitting existing happy-dom stylesheet warnings. The focused changed-test suite remained green.

## Risks

- Advertised ACP load or resume support can still fail for a changed server
  list. Runtime state must reflect the request result, not capability metadata.
- Global profile migration can multiply one legacy configuration across several
  workspaces. Deterministic import keys and per-pair transactions are required.
- Legacy JSON can contain literal credentials. Migration and diagnostics must
  preserve behavior without exposing those values.
- The public Registry is a preview service without uptime or durability
  guarantees. Last-good-cache behavior is part of the first release.
- Multi-repository tasks can select the same definition through several paths.
  Identity-based deduplication must retain every origin.
- Deep marketplace forms can regress mobile scrolling if they reuse the desktop
  pane structure.
- `session/load` resets more than the MCP list. Using it as an MCP-only
  reconnect silently drops armed wakeups unless that cost is preserved or
  disclosed.
- An existing `SupportsSessionResume` registry flag shares the name of the ACP
  capability but not its meaning, and is set on nearly every agent.
- Delivery necessarily exposes bound secrets inside the task executor. The
  product must state that boundary rather than imply end-to-end secrecy.
