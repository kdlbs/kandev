---
created: 2026-09-28
status: complete
requirements:
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-001
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-002
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-003
system_design:
  - ../../specs/agents/system-design/cursor-plugin-mcp-import.md
legacy_specs: []
---

# Implementation plan: Cursor plugin MCP import

## Overview

Import supported installed Cursor plugin and user MCP definitions into local Cursor task launches with a persistent per-profile opt-out. Requirements and design remain draft. Review is against workspace documents over `adc5d67f55d19dd76161eaec5ae725e20a345acb`; no production implementation is present.

First resolve versioned compatibility and import ownership evidence, then implement discovery and profile contracts, integrate launch/reconciliation, and finish the settings UI and end-to-end evidence. Execute sequentially. Task 00 gates production implementation because cache contents do not prove installation and existing additive merge/cleanup cannot satisfy disable/restart semantics.

Sources: [requirements](../../specs/agents/requirements/cursor-plugin-mcp-import.md), [system design](../../specs/agents/system-design/cursor-plugin-mcp-import.md), and [ADR 0014](../../decisions/0014-passthrough-mcp-injection-strategies.md). The shipped [Cursor OAuth bridge package](../cursor-mcp-oauth-bridge/plan.md) is a compatibility dependency, not pending work to reimplement; keep its independent preference semantics unchanged.

## Scope

In scope: eligible source selection, supported manifest parsing, precedence/policy, profile persistence, both Cursor launch modes, owned-entry reconciliation, localized settings and focused verification.

Out of scope: installing plugins, marketplace APIs, arbitrary variable/secret resolution, remote credential forwarding, generic CLI refactoring, and unsupported plugin execution fields such as cwd without a separately reviewed contract extension.

## Technical approach

- Discovery: `mcpconfig/cursor_plugins.go` returns provenance-bearing candidates. Installed/enabled/scope selection is evidence-backed, not a recursive cache scan. See the design's compatibility table for supported and skipped shapes.
- Composition: resolve conflicts before `CursorStrategy` emission; explicit profile > user-owned project > global > plugin, with `kandev` reserved. Apply MCP executor/transport policy to imports. Never append imports so that last-write-wins serialization replaces profile definitions.
- Profile: follow the existing auth preference through schema/rebuilds, DTO omission semantics, controller duplication, runtime execution-profile resolution, catalog generation, browser normalization, save and conflict reconciliation.
- Materialization: share one Cursor-only lifecycle preparation helper between ACP and terminal paths. Use the Task 00 ownership design for stale-import removal and generation-safe cleanup rather than generic merge alone. Preserve unrelated project configuration and user edits.
- UI: reuse the profile settings surface; Task 04 owns rendering, localized helper copy, public guide and browser evidence.

## ASCII UI preview

### UI-01: Cursor profile import preference

Entry: Settings > Agents > Cursor profile, enabled state. Shared desktop/phone structure.

```text
Profile settings
  ...existing fields and credential preference...
  [x] Import local Cursor MCP servers
      Import plugin and user MCP definitions
      on the next local launch.

  [Discard]  [Save changes]
```

Unchecked draft uses `[ ]`; discard restores the saved value. Saving disables the existing actions; failure preserves the draft and shows the existing localized error. After reload the saved value remains. Non-Cursor profiles omit this row.

Reuse the existing settings page scroll owner, navigation and safe-area handling. Phone helper text wraps and the actual label hit target measures at least 44px; desktop keeps existing density. No new overlay or fixed region. Keep limitations visible in helper copy: copying only, independent from credential sharing and native Cursor global discovery. Required structure is illustrative, final copy is localized. Maps to AC-AGENTS-CURSOR-PLUGIN-MCP-002.2 and .4.

## Tests

New test names below are planned. Each work order writes a failing regression before changed logic.

| Acceptance criteria | Planned evidence |
| --- | --- |
| 001.1-.4, .7-.8 | `mcpconfig/cursor_plugins_test.go`: `TestDiscoverCursorPluginMCPServers`, `TestCursorPluginMCPPathSafety`; Task 00 versioned fixtures |
| 001.5-.6 | Same file: `TestCursorPluginMCPPrecedence`; lifecycle `TestCursorPluginMCPPolicy` including explicit-profile name reservation |
| 001.9 | Lifecycle `TestCursorPluginMCPPolicy` plus parser source-immutability and redaction cases |
| 002.1, .5 | Store/controller CursorPluginsMCP tests, `TestStoreProfileResolver`, catalog check, browser normalization/save/reconciliation suites |
| 002.2, .4 | `profile-form-fields.test.tsx` and desktop/phone E2E |
| 002.3, 003.2, .5-.6 | `cursor_plugin_mcp_test.go`: `TestCursorPluginMCPReconciliation`, `TestCursorPluginMCPConcurrentCleanup`, `TestCursorPluginMCPCrashRecovery` |
| 003.1, .3-.4 | `TestCursorPluginMCPEligibility`, `TestCursorPluginMCPLaunchPaths`, project/passthrough integration tests and emitted transport fixtures |

All IDs in this table have prefix `AC-AGENTS-CURSOR-PLUGIN-MCP-`. Work-order frontmatter expands the exact identities. All tests use disposable fixtures. No tests read personal MCP credentials.

## E2E tests

`tests/settings/cursor-plugin-mcp.spec.ts` (chromium) and `tests/settings/mobile-cursor-plugin-mcp.spec.ts` (mobile-chrome) prove default/disable/save/reload/discard and profile applicability (002.1-.4). The desktop file also launches a mock local task with an isolated HOME, checks materialized imports, then disables and relaunches to prove cleanup (003.1/.3, 002.3). Phone coverage measures actual touch geometry and checks overflow and localized wrapping. A mock cannot prove actual Cursor tool registration: Task 00 records versioned CLI evidence, and Task 03 reruns that smoke with implemented output before claiming compatibility.

## Work orders

- [x] [Task 00: Resolve Cursor compatibility and ownership evidence](task-00-compatibility-evidence.md)
- [x] [Task 01: Plugin MCP discovery engine](task-01-discovery-engine.md)
- [x] [Task 02: Profile preference and store contract](task-02-profile-contract.md)
- [x] [Task 03: Launch integration and owned import reconciliation](task-03-launch-materialization.md)
- [x] [Task 04: Settings UI, localization, and end-to-end evidence](task-04-ui-and-i18n.md)

Dependencies: 00 -> {01, 02} -> 03 -> 04. This graph does not authorize parallel agents; default execution is sequential.

## Verification results

Implementation verification (2026-09-28):

- Backend mcpconfig tests: `(cd apps/backend && go test -race -v ./internal/agent/mcpconfig/...)` passed.
- Backend lifecycle Cursor & Project MCP tests: `(cd apps/backend && go test -race -v ./internal/agent/runtime/lifecycle/... -run "Cursor|ProjectMCP")` passed.
- Backend settings store & controller tests: `(cd apps/backend && go test -race ./internal/agent/settings/store/... ./internal/agent/settings/controller/...)` passed.
- Backend lint: `golangci-lint run ./internal/agent/mcpconfig/... ./internal/agent/runtime/lifecycle/... ./internal/agent/settings/...` passed with 0 issues.
- Frontend typecheck & lint: `pnpm run typecheck && pnpm run lint` passed with 0 errors/warnings.
- Frontend i18n checks: `pnpm run i18n:check` passed across all 7 catalogs.
- Frontend Vitest unit tests: 77 tests passed across profile forms, normalizers, and save helpers.

Design review validation (2026-09-28):

- `python3 scripts/list-docs.py validate`: passed (321 decisions, 1222 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: actual documentation-only snapshot exempt; an additional in-memory preflight with the planned `cursor_plugins.go` path as a synthetic trigger validated all five work orders as covered, with no errors. No source file was created for this check.
- `git diff --check`: passed; because the eight documents are untracked, their whitespace was also checked directly, with zero errors.
- `git status --short`: only this untracked plan directory and the two paired untracked specifications; nothing staged or committed.

Product tests and CLI smoke checks were not run in this documentation review. Do not mark product tests passed from document checks.

## Risks and unresolved evidence

- Installed/enabled/scope metadata and cache layout require Task 00 evidence. Unknown state must not activate cached code by default.
- Cursor ACP behavior and accepted network type serialization vary by CLI version; the original assertion of seamless support was unverified.
- Shared-workspace ownership, crash recovery, and stale cleanup require a concrete persistence design in Task 00 before launch implementation. Existing file-level metadata is insufficient.
- Default-enabled imports can introduce plugin commands and secret-bearing values into task config. Source eligibility, policy, permissions, redaction and cleanup are mandatory, not optional follow-ups.
- The opt-out stops Kandev copying only. Cursor can still read its own global configuration; public/UI copy must not promise complete MCP disablement.
