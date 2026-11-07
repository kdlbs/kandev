---
created: 2026-11-04
status: completed
requirements:
  - REQ-OFFICE-AUTOMATION-MCP-001
  - REQ-OFFICE-AUTOMATION-MCP-002
  - REQ-OFFICE-AUTOMATION-MCP-003
  - REQ-OFFICE-AUTOMATION-MCP-004
system_design:
  - ../../specs/office/system-design/automation-mcp-selection.md
legacy_specs: []
---

# Implementation plan: Automation MCP selection

## Overview

Add built-in package and workspace-server selection to native automation configuration.
All optional packages are eligible for private runs, including Configuration. Core remains required.
Implement four sequential work orders. Persistence precedes runtime use, and the editor follows verified runtime support.

The user authorized implementation with “go for impl” after the design checkpoint.
No delegation is authorized. The work orders run in the primary session by default.

## Scope

Include native save/read contracts, private-run binding, normal-task templates, additive workspace MCP delivery, UI, locales, and public documentation.
Preserve existing plugin approval and managed-conversation contracts.
Exclude arbitrary native tool allowlists, new packages, new providers, automatic restart, and general MCP navigation redesign.

## Source and related packages

Baseline: `74abd2c92887`. The direct report is [discussion #4997](https://github.com/kdlbs/kandev/discussions/4997).
Its plugin support is already present. [Issue #3805](https://github.com/kdlbs/kandev/issues/3805) is related and closed.

Read the [requirements](../../specs/office/requirements/automation-mcp-selection.md),
[design](../../specs/office/system-design/automation-mcp-selection.md), and
[ADR](../../decisions/2026-11-04-automation-mcp-selection.md).
Related completed/source packages are [built-in MCP management](../builtin-mcp-management/plan.md),
[automation plugin tools](../automation-plugin-tools/plan.md), and
[MCP assignment](../mcp-assignment-after-registration/plan.md).
Their implementation records remain historical. This package owns the new Automation extension.
At implementation start, recheck their base revisions and current contracts before editing shared code.

## Technical approach

1. Extend `internal/automation` models, migration, native handlers, service, and store with typed selection and conditional saves.
2. Extend `internal/mcp/profile`, `server`, `handlers`, executor preparation, and runtime carriers with an Automation package binding.
3. Extend existing workspace resolution inputs with a trusted automation contribution. Preserve normal-task customization and accepted execution evidence.
4. Extend the existing web automation editor and run detail. Reuse shared package descriptors, workspace selection controls, and Save/Discard ownership.

The exact field semantics, authority checks, application timing, and target-mode behavior are in the design.
Do not build a second MCP transport, credentials store, or plugin approval system.

| Runtime path | Required evidence | Unsupported behavior |
| --- | --- | --- |
| Structured ACP/native adapters | Manifest round trip, exact injected package names, successful optional tool call | Fail explicit package launch |
| CLI/project configuration | Existing builder tests preserve exact routes and selected server set | Report unsupported delivery |
| Local/worktree and supported remote helpers | Capability negotiation and prepared-workspace binding | Upgrade guidance before start |
| Legacy retained execution | Same-process continuation preserves old tools and Unknown evidence | No automatic restart |
| Third-party stdio/HTTP/SSE | Existing transport policy, duplicate elimination, secrets, failure projections | Existing filtered decision or error |

Mock and adapter tests prove the shared path, not connectivity for every installed provider.

## UI preview

The structured preview in the task conversation uses these stable view labels.
These descriptions are the durable layout contract. They replace ASCII sketches as requested by the task's rich-output instructions.

**UI-01: Desktop editor, explicit private selection.** Entry: workspace Automations > create/edit.
The existing Then section precedes MCP access. Its groups appear in this order:

1. Built-in packages: Core (Required), Configuration, Rich output, Walkthroughs, Diagnostics.
2. Workspace MCP servers: search, inherited rows with source labels, then additional selectable definitions.
3. Plugin tools: existing grouped selection and approval state.

Package rows share the task/workspace list, status badges, and inline detail view. Open a package to choose On or Off. Configuration's explanation stays visible below the picker. The private timing notice says changes apply at the next agent process start.
Existing Save changes and Discard controls remain in the settings save surface.
Sample package choices are illustrative. Group order, required Core, authority notice, and shared save ownership are required.

**UI-02: Phone editor, same selection.** Use the existing full-page automation editor and inline groups.
The hierarchy matches UI-01. One page scroll owner contains the sections, with the existing save surface clear of safe areas.
Use wrapped labels and at least 44px phone/coarse-pointer targets. Preserve one draft during desktop/phone transitions.
A long workspace catalog uses the existing bounded searchable list. Do not introduce nested dialogs or a new management page.

**UI-03: Recovery and run evidence.** Loading preserves saved choices. An options error offers Retry and retains unavailable selected rows.
A stale save retains the draft and offers Reload choices before an explicit retry.
A saved package change on a reused process shows Desired and Current process separately, plus next-process timing.
Missing historical evidence says Unknown. An On badge never implies Connected.

UI-01/02 map to AC-OFFICE-AUTOMATION-MCP-004.1, .2, .4, and .5.
UI-03 maps to AC-OFFICE-AUTOMATION-MCP-003.1, .4, .5 and AC-OFFICE-AUTOMATION-MCP-004.2, .3.

## Tests and traceability

The following files map the criteria to executable evidence.
Each work order adds tests first through `/tdd`, observes the intended failure, and then implements the behavior.

| Criteria | Planned executable evidence |
| --- | --- |
| 001.1, 001.3-.6, 002.3 | `internal/automation/mcp_selection_test.go`: `TestAutomationMCPSelectionSave`, `TestAutomationMCPSelectionDefaultsAndExport`, `TestAutomationMCPSelectionNativeOnly` |
| 001.2, 003.3, 003.5 | `internal/agent/mcpconfig/automation_selection_test.go`: `TestAutomationMCPSelectionUnion`. `internal/orchestrator/automation_mcp_selection_test.go`: `TestAutomationMCPNormalTaskTemplate` |
| 002.1, 002.2, 002.5, 002.6 | `internal/mcp/server/automation_packages_test.go`: `TestAutomationMCPPackageInvocation`. `internal/mcp/handlers/automation_packages_test.go`: `TestAutomationMCPRawDispatch` |
| 002.4 | `internal/automation/mcp_selection_test.go`: `TestAutomationMCPConfigurationGrantDenial` |
| 003.1, 003.2, 003.4 | `internal/orchestrator/executor/automation_mcp_selection_test.go`: `TestAutomationMCPBindingLifecycle`. runtime recovery tests extend that scenario |
| 004.1-.5 | Web payload, hook, editor, and run-evidence tests plus UI-01/02/03 E2E below |

Numeric criteria in this table use prefix `AC-OFFICE-AUTOMATION-MCP-`.
SQLite and real PostgreSQL tests cover migration, stale-save rejection, atomic state, and automation reference counts. Catalog tests cover guarded and confirmed deletion.
Add new Go test names with `AutomationMCP` so the work-order commands discover them.
Record discovered counts and skipped tests. A skipped PostgreSQL suite is not successful database coverage.

## E2E tests

Add `apps/web/e2e/tests/automations-mcp-selection.spec.ts` (chromium) and
`apps/web/e2e/tests/settings/mobile-automations-mcp-selection.spec.ts` (mobile-chrome).

- Save all four optional packages and an additional workspace server. Reload and verify persistence and exact invocation in a mock-backed private run.
- Save while a private process runs and verify separate desired and accepted packages. The next firing reuses the conversation and starts a process with the new selection. The private lifecycle stops the previous process at turn completion.
- Create a normal task with a template. Edit its task choices and reuse it without overwriting those edits.
- Cover loading, options failure, stale responses, unavailable resolution, and stale saves through domain/repository tests. Exercise stale-save recovery and independent plugin approval in the browser.
- On phone, complete Save and Reset and preserve the draft through viewport changes. Measure targets and document overflow at 320px, 393px, 767px, and 768px. Cover narrow fine-pointer keyboard access. Domain tests cover request recovery for both layouts.

Use existing automation, built-in-package, and plugin-tool fixtures. Do not probe real third-party endpoints.

## Work orders

- [x] [Task 01: Persist native automation selections](task-01-selection-contract.md)
- [x] [Task 02: Bind and enforce native packages](task-02-native-packages.md)
- [x] [Task 03: Deliver workspace servers and task templates](task-03-workspace-delivery.md)
- [x] [Task 04: Add editor, run evidence, and documentation](task-04-editor-and-docs.md)

Dependency order: 01 -> 02 -> 03 -> 04. Work orders run sequentially in the primary session.
Each work order contains runnable targeted commands. Install workspace dependencies once before the first pnpm command.
Run changed-Go lint through `scripts/lint-go-changed` for the actual changed Go paths after each Go work order.

## Verification results

Design checks completed on 2026-11-04:

- `python3 scripts/list-docs.py validate`: passed (430 decisions, 1910 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: actual documentation-only diff is exempt. A preflight with the planned model change validates all four work orders as covered.
- ASD-STE100 structural lint across the eight new documents: passed with zero hard errors. Synonym rotation remains advisory for explanatory prose.
- `git diff --check`: passed. New-file whitespace and Markdown target checks also passed.

Implementation checks on 2026-11-04:

- Backend work-order suite: 380 passing cases (210 top-level tests), 27 successful packages, zero failures or skipped tests. PostgreSQL 17 ran with UTF8 encoding. This includes actual optional-package handlers, direct-dispatch checks, native grant denial, SQLite/PostgreSQL save conflicts, resolver snapshots, helper capabilities, and continuation/retry contracts.
- Full affected package suites also passed after correcting test fixtures. The full automation and settings-store suites passed against the isolated PostgreSQL service. Export disposition and portable grant omission passed.
- Race checks: `GOMAXPROCS=2 go test -tags fts5 -trimpath -race -p=2 ./internal/automation ./internal/orchestrator/executor -run AutomationMCP -count=1`: passed.
- Web verification command in Task 04: 80 tests passed in eight files. The existing built-in attachment and plugin domain regressions are included.
- Web typecheck passed with `NODE_OPTIONS=--max-old-space-size=8192`. The default Node heap was insufficient for this repository.
- Full web lint, `i18n:check`, and `i18n:ratchet`: passed. All seven locales and pseudo were checked.
- Desktop: all four new scenarios passed. Existing plugin-tool selection and both target-mode regressions passed in the preceding combined run.
- Phone: all three selected scenarios passed. They include the existing plugin flow, all-package Save/Reset, 320/393/767/768px overflow and draft retention, 44px targets, and narrow fine-pointer keyboard access.
- Desktop and phone MCP sections were captured and visually inspected. No actionable UI-polish findings were found. No new animation was added.
- Public docs: 63 validator tests passed. All 55 published pages validated. The automation guide is a how-to guide. The package page is a reference.
- Documentation coverage validated all four work orders against the actual runtime diff. Specification catalog and lint passed.
- Changed-Go lint: passed. The full affected-package run reported one string-constant finding. The final automation/streams rerun passed after the switch kept the existing scope literals on their unchanged line. No behavior changed.
- Final regression rerun after the last production edit: 37 cases passed in ten packages, including both real PostgreSQL cases. No cases failed or skipped. The first rerun found that the disposable database had stopped during the session pause. Restarting that database resolved the setup failure.

The following command ran the combined backend work-order suite from `apps/backend`, with `KANDEV_TEST_POSTGRES_DSN` set to the isolated database:

```bash
go test -tags fts5 -trimpath -json ./internal/automation ./internal/mcp/profile ./internal/mcp/server ./internal/mcp/handlers ./internal/mcp/scope ./internal/orchestrator/executor ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/... ./internal/agentctl/types/streams ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/sysprompt/... -run 'AutomationMCP|AutomationPluginTools|Builtin|Automation.*Retry|Automation.*Continuation|MCP.*Setup|PostgresMCPResolution|PostgresConfirmedMCPDelete' -count=1
```

Browser commands ran from `apps/web` with inherited Kandev service settings removed, an unused port offset, and the host Chromium library path. The runner built the backend, fixture binaries, and web assets before the final `--no-build` runs:

```bash
pnpm e2e:run --host --no-build --project chromium tests/automations-mcp-selection.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-automations-mcp-selection.spec.ts tests/settings/mobile-automation-plugin-tools.spec.ts
```

The private fixture stops its process on turn completion. The browser timing test therefore saves during a live turn, checks that accepted packages stay fixed, and checks the next process on the same task/session. Prepared/retained binding tests cover the shared continuation path.
Mock-backed execution and existing adapter tests do not certify live third-party MCP endpoints or every installed provider.

Confirmed catalog deletion preserves automation references as unavailable. It does not attempt an atomic update across separately owned stores. Native Save removes those references explicitly. The design records this refinement.

The expanded affected-package lint invocation was:

```bash
golangci-lint run ./internal/agent/mcpconfig ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/agent/settings/store ./internal/agentctl/server/api ./internal/agentctl/types/streams ./internal/automation ./internal/backendapp ./internal/mcp/handlers ./internal/mcp/profile ./internal/mcp/scope ./internal/mcp/server ./internal/orchestrator ./internal/orchestrator/executor ./internal/sysprompt --new-from-rev=74abd2c92887e3b7b7d6a14eb803067e4e0f9565 --timeout=5m --concurrency=2
bash scripts/lint-go-changed apps/backend/internal/automation/mcp_selection_test.go apps/backend/internal/agentctl/types/streams/mcp_resolution.go
```

The wrapper supplies the installed `golangci-lint` through PATH and bounds Go memory and workers. An initial cold-cache lint attempt exceeded the loading timeout. The complete rerun reached analysis. The final two-package fix rerun reported zero issues.

The user authorized a ready-for-review PR on 2026-11-05. Publication uses the repository template and fresh desktop/phone screenshots.

## PR remediation

PR #5106 remediation runs in the primary session. The base merge preserves both new helper capabilities and automation selection.

The first review batch identified 41 findings. Changes cover resource-key and profile ownership checks, helper negotiation before preparation, retry templates, native question tools, batched usage and HTTP deletion guards, editor reconciliation and removable stale choices, translated origins, and stronger regressions. The shared requirement keeps its stable ID and gains separate UI criteria.

Local verification on 2026-11-06:

- The expanded backend command uses `-tags fts5`, the Task 04 package set plus agent-settings handlers, and the existing isolated PostgreSQL database. It passed 987 cases across 28 packages. One host-ownership case skipped because `chown` is unavailable. Provider E2E and Windows-only packages remain platform-gated.
- The focused web command adds the origin-label and origin-group tests. All 116 tests passed in ten files, including reconciliation, mode changes, and origin projection.
- Web typecheck, lint, translation integrity, and the new-copy ratchet passed. Specification structure and catalog checks passed.
- Changed-Go lint passed after extracting the two resource-field constants. The full affected-package scan reported only those findings. The final handler-package rerun reported zero issues.
- All eight desktop and four phone scenarios passed, including blocked-package removal, inherited/direct server removal, and keyboard focus across viewport changes. Both used the managed runner and fresh application/fixture builds.
- The first remediation commit passed the normal hooks and was pushed. Live CI, review dispositions, and final screenshot publication are tracked on PR #5106.


The second review batch adds compare-and-swap guards for mode transitions, committed revision responses, workflow-step ownership checks, and the private Configuration write boundary. Historical evidence remains visible when current settings change. Normal-task templates omit unavailable servers and preserve diagnostic reasons without granting those servers. Reloads reject older responses. Browser saves wait for their matching update response.

The CI remediation updates four stale web store mocks. Exact-run browser fixtures now use the real automation trigger and durable run binding. A regression reproduces rejected MCP preparation without losing the caller context during environment cleanup. The focused web checks passed 137 tests in 15 files, plus typecheck, lint, translation integrity, and the new-copy ratchet. The broad seven-package backend run passed 15,106 cases. Two existing process tests failed because this Nix host has no `/bin/sleep`; no product assertion failed. The full automation, MCP configuration, handlers, server, orchestrator, and executor packages passed. Focused lifecycle and launch regressions are checked separately. All nine desktop and four phone scenarios passed with retries disabled. Both exact failing CI stop scenarios also passed. Changed-Go lint passed after the resource-field constants were extracted. Normal hook results and live CI/review completion are recorded on the PR.


A later review follow-up identified a missing mixed-usage assertion in the deletion tests. `TestAutomationMCPHTTPDeleteReportsMixedUsage` now checks the complete HTTP 409 impact across ordinary selection scopes and automations. Restoring the original early return makes this test fail with a missing automation count; the current implementation passes. `TestAutomationMCPTemplateReloadsSavedSelection` uses a separate service instance and the persisted store to check exact package preferences, policy revision, and definition IDs. The orchestrator retry case also checks the forwarded preference contents. All 26 focused cases across the three affected packages passed, and changed-package Go lint reported zero issues.


The fresh review on `ad8c057` completed with all 90 CI checks passing and added one inline finding plus 15 grouped suggestions. The next batch corrects executor-profile ID classification, scopes MCP save recovery to the update request, moves reload ownership changes into committed effects, rejects stale options callbacks, and handles the browser response wait with the pending click. The editor save operation has its own module and stage-specific failure tests. The deletion dialog now explains retained automation selections and the loss of attachment at a new process start in all shipped locales. The support table and targeted test assertions were corrected.

This batch passed 144 web cases in 16 files, 39 focused backend cases across four packages with PostgreSQL enabled, changed-Go lint, web lint/typecheck, translation integrity, and the new-copy ratchet. All ten desktop and five phone browser scenarios passed with retries disabled, including the deletion warning and retained-reference checks. The final CI result and dispositions of grouped suggestions remain recorded on PR #5106.


The incremental review on `10c3e4d` had no inline findings but reported two grouped issues. A Unicode case-fold alias such as `taſk_id` could pass the lowercase-only check and then bind to `TaskID` in the Go decoder. The new regression reproduced that bypass. Payload keys now require lowercase ASCII; Unicode values and nested user content remain valid. The complete MCP handler suite passed 1,184 cases. The automation usage label now uses a localized count key and plural forms in every shipped locale. The deletion browser flow asserts the singular label. All ten desktop and five phone flows passed after a fresh application build, including all native package invocations. Web lint/typecheck, translation integrity, the new-copy ratchet, and changed-Go lint passed.

## Risks

- Discovery-only changes leave direct dispatch broken. Test one successful call for every optional package.
- Configuration expands authority. Preserve Automation identity and test all generic grant-write paths.
- A reused process does not automatically adopt changed selections. Timing must remain visible.
- Old helpers reject new binding versions. Keep legacy behavior and explicit upgrade errors distinct.
- Normal-task template application must not overwrite later task edits or start an agent after a partial application.
