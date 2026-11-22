---
created: 2026-11-02
status: in_progress
requirements:
  - REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-001
  - REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-002
  - REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-003
system_design:
  - ../../specs/office/system-design/automation-plugin-tools.md
legacy_specs: []
---

# Implementation plan: Private automation plugin tools

## Overview

Add an optional plugin tool selection to the existing automation editor.
Saving the changed selection approves exact tools for one automation, workspace, installation, and plugin policy version.
Keep the fixed core MCP surface and hidden run history.

Two sequential work orders deliver the backend boundary and then the editor with browser evidence.
The [requirements](../../specs/office/requirements/automation-plugin-tools.md), [design](../../specs/office/system-design/automation-plugin-tools.md), and [decision](../../decisions/2026-11-02-automation-plugin-tool-selection.md) form the design package.
Implementation is complete in code. The PostgreSQL and managed Playwright closure gates remain pending until those runs are recorded.

## Scope

### In scope

- One saved `plugin_tools` list on each automation.
- One persisted policy version on each installed plugin.
- Explicit plugin declaration of the existing `automation` surface string.
- Native editor save, filtered discovery, and per-call authorization.
- Inline desktop/phone controls, translated copy, and targeted tests.
- An in-tree fixture read tool and report-write tool for end-to-end evidence.
- Public automation and plugin-author documentation during implementation.

### Out of scope

- An approval ledger, queue, dialog, or permission settings page.
- Generic plugin actions, wildcard selections, or core-tool selection.
- New task modes, scheduler behavior, runtime flags, or run statuses.
- New SDK/protobuf fields or persisted execution policy snapshots.
- Changes to ordinary task and managed-conversation authority.
- Third-party plugin changes or releases, including Kandev Security.

## Technical approach

1. Persist and migrate the selection in `internal/automation/models.go` and `store.go`.
   Put the shared selection type in `internal/mcp/plugintools`.
   Preserve omitted updates and exclude authority from portable export.
2. Add `AgentToolPolicyVersion` to `plugins/store.Record`.
   Initialize it in install/load/sideload paths and replace it before admitted configuration changes.
   Reuse existing plugin lifecycle and dispatch locks.
3. Wire the narrow plugin options/validation dependency through `internal/backendapp`.
   Native automation handlers mark configuration intent internally.
   MCP and plugin schedule callers cannot approve tools.
4. Extend manifest and snapshot surface validation.
   Filter the list with the trusted principal and current automation row.
   Check the current selection again before each plugin RPC.
5. Treat global catalog pushes as invalidation for automation sessions.
   Reuse the before-list backend request for the approved catalog.
   Keep ordinary pushed/pulled snapshots compatible with older surface validators.
6. Add the inline editor section, domain hook, request types, dirty handling, and locale copy.
   Prove real MCP read/write calls from a hidden run with the packaged fixture.
7. Reconcile the earlier automation/plugin contracts only when their implementation changes.
   Update public documentation with the delivered behavior.

### Compatibility matrix

| Surface or consumer | Expected behavior | Evidence |
| --- | --- | --- |
| Private automation, updated agentctl | Exact approved plugin tools plus unchanged core tools | Backend authorization tests and desktop hidden-run E2E |
| Ordinary task or managed conversation | Existing authority and catalogs remain intact | MCP/manifest regression tests |
| Global catalog push | Never adds an unapproved automation tool | MCP server regression test |
| Client without live refresh | Refresh on next list/reconnect, with stale calls denied | Direct stale-call test |
| Older retained agentctl | Core automation continues, new tools require execution upgrade | Legacy-safe catalog projection test |
| Plugin without `automation` declarations | No automation options or access | Manifest/options tests |
| Kandev Security | Author adds the surface to intended declarations and handler | External compatibility note, not a completed implementation claim |

## ASCII UI preview

### UI-01: Plugin tools in the automation editor

Entry: Settings > workspace > Automations > private automation editor.
Place the section with the existing configuration controls.

```text
Desktop and phone: existing editor scroll surface

Plugin tools (optional)
Select the tools this automation can use.

Kandev Security                         Clear
[x] List findings                  Read
    Read findings in this workspace.
[ ] Complete report                Write
    Publish a report from this automation.

Existing editor Save / Discard controls
```

Both viewports use the same inline hierarchy and shared draft state.
On phones, labels and descriptions wrap within full-width rows.
The complete selection row and Clear action provide 44-pixel touch targets.
Ordinary desktop actions retain the existing 28-pixel size.
The existing editor owns scrolling and save controls.
There is no extra dialog, drawer, or nested scroll area.

### UI-02: Review required

```text
Plugin tools (optional)
Kandev Security                         Clear
Review required: the plugin configuration changed.
Previously selected: List findings

[ ] List findings                  Read
[ ] Complete report                Write

Select tools again and save to restore access.
```

The prior selection remains stored but grants no access.
Current choices start unchecked and require explicit interaction.
An unrelated Save omits the field and does not approve or clear it.

Loading, no-compatible-plugin, and load-error states use visible inline text.
An unavailable saved plugin keeps its previous names and a Clear action.
These states must preserve other editor changes.
Names and spacing are illustrative.
Grouping, explicit selection, existing Save ownership, touch access, and stale behavior are required by AC-003.1 through AC-003.5.

## Tests

All new tests follow TDD during implementation.
These names reflect the implemented tests. Browser and PostgreSQL closure gates remain pending.

| Acceptance criteria | Evidence |
| --- | --- |
| 001.1, 001.3, 001.5, 001.6 | `internal/automation/plugin_tools_test.go`: `TestAutomationPluginToolsPersistence`, `TestAutomationPluginToolsModeChangeClearsSelection`; `plugin_tools_postgres_test.go` covers replay and omitted updates |
| 001.2, 001.7 | `internal/automation/plugin_tools_test.go`: `TestAutomationPluginToolsNativeSaveOnly`, `TestAutomationPluginToolsRejectStaleOptions`, `TestAutomationPluginToolsRejectSharedDispatcherMCPMutation` |
| 001.4 | `internal/plugins/agent_tools_automation_test.go`: `TestAgentToolPolicyVersionLifecycle`; `internal/plugins/service_install_test.go` upgrade assertions |
| 002.1, 002.3 | `internal/mcp/handlers/plugin_tools_automation_test.go`: `TestAutomationPluginToolsDiscoveryIsFiltered`, `TestAutomationPluginToolsInvocationScope`; `internal/plugins/agent_tools_automation_test.go`: `TestAutomationAgentToolInvocationRequiresExactCurrentApproval` |
| 002.2 | `internal/mcp/server/server_automation_plugin_tools_test.go`: `TestAutomationPluginToolsGlobalPushCannotWiden`, `TestAutomationPluginToolsSelectionRefreshAtSameCatalogRevision`; `server_automation_plugin_tools_refresh_test.go`: delayed-response and invalidation tests |
| 002.4 | Existing fixed automation catalog tests plus the shared MCP dispatcher regression |
| 002.5, 002.6 | Fixture read/write implementation plus the desktop hidden-run Playwright spec |
| 003.1, 003.2, 003.4 | `components/automations/automation-plugin-tools.test.tsx`, `automation-payload.test.ts`, and `hooks/domains/settings/use-automation-plugin-tools.test.ts` |
| 003.3 | `apps/web/e2e/tests/settings/mobile-automation-plugin-tools.spec.ts` measures touch targets and horizontal overflow |
| 003.5 | Locale checks plus pseudo catalog completeness |

IDs in this table use the `AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-` prefix.
Keep full IDs in work orders and test coverage annotations where needed.

The central upgrade regression starts with approved reads.
An update adds a tool and changes the policy version.
The next call denies the old approval, and the new tool never appears as approved.
After explicit read-only reselection, the added tool remains denied.

## E2E tests

- `e2e/tests/automations-plugin-tools.spec.ts`, project `chromium`: default-empty access, editor selection, save/reload, and actual hidden-run read/report submission.
  Use `cmd/mock-agent/script.go`'s existing `e2e:mcp:` command through the real MCP server and packaged plugin.
  Assert the report receipt and run transcript, with no generated task in Kanban or sidebar.
- The same desktop spec covers clear-access, plugin configuration change, review/reselection, stale editor rejection, and the upgrade regression.
  Use an isolated disposable backend/plugin installation and existing causal waits.
- `e2e/tests/settings/mobile-automation-plugin-tools.spec.ts`, project `mobile-chrome`: tap selection, Save, reload, review, and clear.
  Measure actual 44-pixel targets and document containment.
  Use long translated descriptions and the pseudo locale for the changed region.

The specs are intended to cover AC-001.1, AC-001.2, AC-001.4, AC-001.5, AC-002.5, and AC-003.1 through AC-003.5.
They have not yet been executed in the managed runner, so browser evidence remains a closure gate.
Backend tests own forged identities, raw denied actions, and failed authority reads.

## Work orders

- [x] [Task 01: Persist and enforce selected plugin tools](task-01-selection-and-enforcement.md)
- [x] [Task 02: Add editor controls and end-to-end proof](task-02-editor-and-proof.md)

Execute sequentially, without delegated agents.
Task 02 depends on Task 01's native options/save and MCP contracts.
No existing package is reopened.
The completed [target-mode package](../automation-target-modes/plan.md) remains historical input.

## Verification results

Implementation checks on November 3, 2026:

- Backend: `go build ./...` passed in `apps/backend`.
- Backend tests passed for `internal/automation`, `internal/plugins`, `internal/plugins/manifest`, `internal/mcp/plugintools`, `internal/mcp/profile`, `internal/mcp/scope`, `internal/mcp/server`, `internal/mcp/handlers`, `internal/backendapp`, and `cmd/plugin-fixture`.
- `bash scripts/lint-go-changed` over every changed and new Go file: 0 issues.
- Web: targeted Vitest files passed; `tsc --noEmit` passed; ESLint over changed files passed with no warnings; `i18n:check` and `i18n:ratchet` passed.
- Added desktop and phone Playwright specs. The full managed E2E suite was not executed in this environment because it requires the staged plugin package and a full local backend/web build.
- Public documentation updated for automation plugin tools and the plugin author opt-in surface.

Design-package checks on November 2, 2026:

- `python3 scripts/list-docs.py validate`: passed, including both new specification files.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs`'s `validateCoverage` local preflight: passed for both work orders and their planned backend/frontend paths.
- `git diff --check -- docs/specs docs/decisions docs/plans/automation-plugin-tools`: passed.
- Direct whitespace checks include all six untracked package files and passed.

## Risks

- A global catalog push can bypass filtering unless the automation surface treats it as invalidation.
- A secret save can preserve the same vault reference, so configuration hashes do not replace the policy version boundary.
- Form hydration can silently renew stale access unless the tool selection has its own dirty state.
- Kandev Security remains incompatible until its author updates its manifest and handler.
- Older retained agentctl processes require an execution upgrade for the new automation tools.
