---
created: 2026-10-27
status: draft
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-002
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
legacy_specs: []
---

# Implementation plan: Copilot profile plugins

## Overview

Add explicit whole-plugin selection to concrete Copilot profiles using native plugin-directory arguments.
The [requirements](../../specs/agents/requirements/copilot-profile-plugins.md) distinguish user requirements from proposed decisions.
The [design](../../specs/agents/system-design/copilot-profile-plugins.md) owns the runtime, API, security, and UI contracts.

Implementation was authorized on 2026-10-27. Discovery, profile state, UI, and process preparation are implemented, but nonempty process starts are fail-closed with `policy_unverified` until native policy evidence exists. [compatibility-evidence.md](compatibility-evidence.md) records the reproduced inventory/mount work and the missing authenticated veto evidence. WO-00, WO-03, WO-04, and WO-06 remain pending; WO-01, WO-02, and WO-05 are completed. WO-05's browser acceptance ran with a disposable E2E Copilot identity, and its completed scope is the settings UI only.

## Scope

### In scope

- CLI inventory, qualified metadata resolution, and safe profile-aware discovery.
- Typed profile state across storage, HTTP/MCP settings, browser drafts, duplication, and generated contracts.
- Native mounts for local/worktree Copilot ACP and profile-owned terminal/utility/probe processes.
- Copilot-only inline UI, phone parity, translated copy, and targeted desktop/mobile E2E coverage.
- Public documentation and versioned native acceptance evidence.

### Out of scope

- Global activation changes, plugin management, MCP extraction, credential copying, and new permission bypasses.
- Remote plugin transfer or portable-config expansion.
- Exclusive plugin filtering, hot reload, automatic session termination, and a runtime pin change.
- Cursor changes, generic selector abstractions, and a new feature toggle.

## Assumption check and design decisions

No unanswered product question blocks drafting. The choices below are proposals for the package review, not additional confirmed user requirements.

| Topic | Decision |
| --- | --- |
| Default | Empty set on old and new profiles |
| Semantics | Add selected plugins to native loading, without an exclusive whitelist claim |
| Missing selection | Keep it visible/removable, allow repair saves, block a new process with an action |
| Sources | Metadata-backed installed cache/direct/live roots only when qualified for the selected runtime |
| Built-ins | Visible but unavailable without a supported filesystem root |
| Executors | Backend-owned local/worktree only, clear rejection elsewhere when nonempty |
| Manual flags | Preserve order, suppress an equal typed root, reject a conflicting root for the same plugin |
| Execution paths | ACP fresh/promotion/replacement/new-process recovery, passthrough, profile utility and profile probes |
| Existing process | Keep original mounts until another process starts |
| Preview | Base command plus a deferred-mount notice, without exposing host directories |
| Compatibility | Qualify selected packages without falling back to a global CLI or changing the pin |

## Technical approach

### Baseline and ownership

Planning inspected clean commit `a6144c3cb8ef586b7e56f904fae9acaf38569113`.
The selected default package is `@github/copilot@1.0.88`.
The handoff's successful `1.0.103/1.0.104` observations came from another machine.
No native runtime compatibility was exercised during planning.
Agents owns this capability because concrete profiles and provider process configuration are its durable contracts.
No system README change is needed because ownership remains unchanged.

### Discovery and directory adapter

Add `internal/agent/copilotplugins` with a bounded native command runner, strict inventory parsers, and a narrow JSONC state adapter.
Use the selected `ManagedNPMRuntimeSpec.InteractiveCommand`, profile environment, and verified config-directory precedence.
Add authorized `POST /api/v1/agents/:id/plugin-discovery`, safe DTOs, and scoped cache/request fencing.
Never return roots or raw native state to the browser.

Work order 00 qualifies supported package/schema combinations before production adapters rely on them.
The `cache_path` field is a candidate compatibility mapping, not a promised upstream schema.
Unknown sources stay visible and unavailable. Native `enabled: false` is not an exclusion rule.

### Durable profile preference

Add `copilot_selected_plugins`, Go `CopilotSelectedPlugins`, and browser `copilotSelectedPlugins`.
Extend `dto.ProfileCreateRequest`, `ProfileUpdateRequest`, `ProfileContractFields`, controller conversion/duplication, and runtime profile resolution.
Use an atomic supplied-field write so an omitted update cannot overwrite a concurrent explicit selection.
Update fresh schema, migration/replay, table rebuild/copy, shared insert/update/select/scan, and persistence upgrade fixtures.

Extend HTTP/MCP settings, boot/events, browser normalization, agent-level saves, dirty comparison, and conflict reconciliation.
Regenerate catalog outputs with `go run ./cmd/settings-catalog`.
The domain-owned `settings/catalog/catalog.go` is authoritative. Shared defaults are not sufficient alone.

### Process boundaries

Carry stable IDs with one authorized profile snapshot and resolve roots after final environment composition.
Integrate at `configureAndStartAgent`, with coverage for `buildAgentCommandWithContext`, workspace promotion, and `buildFreshAgentCommandWithProfile`.
Apply the same provider adapter to `manager_passthrough.go`, `ExecuteProfilePrompt`, and profile capability/model probes.
Copilot passthrough currently uses an unversioned base. Nonempty typed selections use the selected managed interactive command instead.
An empty list takes the current path without a plugin command or metadata read.
Retained adoption does not reconfigure or restart a process.

### UI and preview

Create `CopilotPluginProfileSelection`, a small selection-row helper, and a domain discovery hook.
Wire the saved profile page and concrete profile creation/edit flow.
Reuse current Save/Reset, draft/conflict state, settings scroll owner, and mobile direct navigation.
Keep a dedicated Copilot component rather than expanding the Cursor MCP component.
Add a deferred-plugin note beside the base command preview.

### Compatibility matrix

| Provider/path | Transport/context | Identity shape | Intended behavior | Evidence/check | Unsupported fallback |
| --- | --- | --- | --- | --- | --- |
| Copilot selected package | Local ACP stdio | Installed marketplace ID | Repeated native mounts | WO-00 and WO-03 native/argv fixtures | Typed unsupported or preparation error |
| Copilot direct/live source | Same local identity | Empty or named marketplace | Exact metadata root, including external live root | WO-00/01 metadata fixtures | Unavailable row, retained preference |
| Copilot built-in | Native inventory row | Built-in ID | Show reason, disable new selection | WO-01 parser and WO-05 UI | `builtin_not_mountable` |
| Copilot isolated executor | Docker/SSH/Sprites/Kubernetes/other | Host profile IDs | Reject nonempty before host access | WO-03 admission tests | `unsupported_executor` launch error |
| Copilot profile utility/probe | Host ACP subprocess | Profile snapshot | Same mounts and context/cache identity | WO-04 tests | Typed error, no default-profile fallback |
| Copilot retained process | Existing ACP connection | Existing process | Preserve old mounts | WO-03 negative side-effect test | No new resolution |
| Agent-wide probe | No concrete profile | No selections | Current behavior | WO-04 negative test | No implicit mount |
| Cursor and other providers | Existing paths | Existing MCP/server settings | Current behavior | WO-02 applicability and WO-05 UI tests | Nonempty Copilot field rejected |

## ASCII UI preview

All views are proposed. Names and English copy are illustrative and must use localization.
Structural requirements are section placement, control order, explicit unavailable states, wrapping metadata, and the existing save flow.
Spacing is not a pixel specification. The fixture `harness` is supplied example data, not a claimed local installation.

### UI-01: Desktop profile, ready and dirty

Entry: Settings > Agents > Copilot > Profile. State: successful discovery, one pending checkbox change.

```text
+------------------------------------------------------------------------+
| Settings > Agents > Copilot > Work profile                              |
+-------------------+----------------------------------------------------+
| Settings menu     | Profile Settings                                   |
| Agents            | Name [Work profile                        ]        |
|   Copilot         | Model [existing model control             v]       |
|     Work profile  |                                                    |
|                   | +------------------------------------------------+ |
|                   | | Plugins *                           [Refresh] | |
|                   | | Load whole plugins for this profile.          | |
|                   | | Plugins can run MCP servers, skills, agents,  | |
|                   | | commands, hooks, and other components.        | |
|                   | |                                                | |
|                   | | [x] harness                                   | |
|                   | |     gst-agentic-platform  1.2.0               | |
|                   | |     Native status: disabled                   | |
|                   | |                                                | |
|                   | | [ ] browser-use                    Unavailable | |
|                   | |     Built-in plugin. Cannot load by directory. | |
|                   | |                                                | |
|                   | | Local/worktree only. Applies to new processes.| |
|                   | | Copilot may load other native plugins.        | |
|                   | +------------------------------------------------+ |
|                   | Command preview                                    |
|                   | [existing base command]                            |
|                   | 1 plugin mount resolves at process start.          |
|                   | Base command excludes this mount.                  |
|                   |                                                    |
|                   |       Unsaved changes     [Reset] [Save]           |
+-------------------+----------------------------------------------------+
```

`*` illustrates the existing dirty treatment, not a required literal character.
The content uses the existing settings scroller. The floating save control clears the last row.
Refresh is a normal 28px desktop action. Whole checkbox labels remain clickable.
Criteria: AC-...-002.4, AC-...-004.6, and AC-...-006.1 through 006.4/006.6.
The full prefix for every abbreviated criterion here is `AC-AGENTS-COPILOT-PLUGINS`.

### UI-02: Phone profile, ready and dirty

Entry: the same profile route through phone Settings navigation.
State: shared draft and inventory from UI-01, focused page without the desktop sidebar.

```text
+--------------------------------------+
| Settings > Copilot > Work profile     |
+--------------------------------------+
| Profile Settings                     |
| Name  [Work profile                ] |
| Model [existing control           v] |
|                                      |
| +----------------------------------+ |
| | Plugins *                        | |
| | [Refresh]                        | |
| | Load whole plugins for this      | |
| | profile, including executable    | |
| | tools, skills, agents and hooks. | |
| |                                  | |
| | [x] harness                      | |
| |     gst-agentic-platform         | |
| |     1.2.0                        | |
| |     Native status: disabled      | |
| |                                  | |
| | [ ] browser-use                  | |
| |     Unavailable                  | |
| |     Built-in. Cannot load by     | |
| |     directory.                   | |
| |                                  | |
| | Local/worktree only. New         | |
| | processes use saved selections.  | |
| | Copilot may load other plugins.  | |
| +----------------------------------+ |
|                                      |
| Unsaved changes    [Reset] [Save]     |
|          existing safe-area gap      |
+--------------------------------------+
```

The existing `settings-scroll-container` is the only content scroller.
The save control keeps its existing safe-area-aware placement. The preview does not add a new toolbar.
Names and metadata wrap. Refresh and checkbox-label targets measure at least 44px on phones and coarse pointers.
At a narrow fine-pointer viewport, phone composition and target sizes still apply.
Criteria: AC-...-002.4 and AC-...-006.5 through 006.7.

### UI-03: Inventory and repair states

These variants replace the Plugins body on both viewports. Existing Save/Reset remains shared.

```text
Loading                      Successful empty
+--------------------------+ +-------------------------------------------+
| Plugins        [Refresh] | | Plugins                         [Refresh] |
| Loading plugins...       | | No installed plugins found.               |
| Saved choices stay shown.| | Install plugins with Copilot, then refresh.|
+--------------------------+ +-------------------------------------------+

Failed refresh, saved selection retained
+-----------------------------------------------------------------------+
| Plugins                                                     [Refresh] |
| Could not refresh plugins. Saved choices are unchanged.                |
| [x] harness  (saved selection, availability not checked)                |
|     You can remove this choice. Refresh before adding plugins.         |
+-----------------------------------------------------------------------+

Missing selected plugin after successful inventory
+-----------------------------------------------------------------------+
| [x] harness                                     Saved, unavailable     |
|     Plugin is missing. Repair its install or uncheck it.                |
|     A new process cannot start with this selection.                    |
+-----------------------------------------------------------------------+

Launch settings changed / runtime unsupported
+-----------------------------------------------------------------------+
| Launch settings changed. Refresh to check plugins in this context.     |
| [Refresh]    Saved choices stay visible and removable.                 |
|                                                                       |
| OR: This Copilot runtime does not support profile plugin loading.       |
| Select a supported runtime in the existing runtime settings.           |
+-----------------------------------------------------------------------+
```

Refresh is disabled while loading and keeps its accessible name. A status region announces progress.
Unavailable unselected rows are disabled. Selected unavailable rows remain removable.
“Availability not checked” does not falsely claim removal after a failed command.
Criteria: AC-...-001.4/001.6, AC-...-003.4/003.5, AC-...-006.2/006.3.

### UI-04: Existing external conflict flow

```text
+-----------------------------------------------------------------------+
| This profile changed elsewhere.                                       |
| [existing conflict recovery actions]                                  |
|                                                                       |
| Plugins: keep the local draft visible while the saved baseline updates.|
+-----------------------------------------------------------------------+
```

This reuses the shipped banner and actions. It does not introduce another conflict dialog.
Criteria: AC-...-002.2/002.4.

## Tests

These are planned test files and test names, not executed evidence.
Test names can gain table-driven subcases without changing the mapped acceptance behavior.

| Criteria | Planned evidence | Work order |
| --- | --- | --- |
| 001.1, 004.1, 005.4, 007.2 | `compatibility-evidence.md`: actual package/binary, format, repeated mounts, native veto matrix | 00 |
| 001.1/001.2/001.4, 003.1 | `copilotplugins/inventory_test.go`: `TestInventoryFormats`, `TestInventoryIdentities`, `TestInventoryRejectsInvalidOutput` | 01 |
| 001.3/001.5/001.6 | Controller/handler `copilot_plugin_discovery_test.go`: `TestCopilotPluginDiscoveryAuthority`, `TestCopilotPluginDiscoveryContext`; package `TestInventoryCommandFailures`, `TestInventoryContextFence` | 01 |
| 003.2 through 003.6, 007.4 | `copilotplugins/installation_test.go`: `TestInstallationMetadata`, `TestPluginRootValidation`, `TestPluginMetadataPrivacy` | 01 |
| 002.1/002.2/002.3/002.5/002.6 | DTO/controller/store `copilot_plugins_test.go`: `TestCopilotPluginSelection`, `TestCopilotPluginSelectionConcurrentOmission`, `TestCopilotPluginSelectionMigration`; MCP/settings-catalog tests with the same prefix | 02 |
| 002.3/002.4/002.6 | Existing normalization/dirty/reconciliation/page-state/save-helper suites extended for the new set | 02 |
| 004.1/004.5, 005.1/005.5 | `copilotplugins/arguments_test.go`: `TestPluginArguments`, `TestPluginLaunchContext`; lifecycle `TestCopilotPluginEffectiveEnvironment`, `TestCopilotPluginManualFlags` | 03 |
| 004.2/004.4/004.7, 005.2/005.3 | Lifecycle `copilot_plugins_launch_test.go`: `TestCopilotPluginLaunchPaths`, `TestCopilotPluginRetainedProcess`, `TestCopilotPluginMCPInjection`, `TestCopilotPluginExecutorAdmission` | 03 |
| 004.3/004.6, 001.6 | Lifecycle `TestCopilotPluginPassthrough`; host utility `TestCopilotPluginProfileUtility`, `TestCopilotPluginProfileProbe`; controller `TestCopilotPluginPreview` | 04 |
| 006.1 through 006.4/006.6, 007.5 | `copilot-plugin-selection.test.ts`, hook `.test.tsx`, selector `.test.tsx`, preview and existing profile-page tests | 05 |
| 006.5/006.7 | Mobile browser geometry, pseudo-locale, complete catalogs, i18n gates | 05 |
| 007.1/007.3 | `TestPluginReadOnlyOperations` in package fixtures and profile mutation tests with command/state spies | 01/02/03 |

All numeric criteria in this table use prefix `AC-AGENTS-COPILOT-PLUGINS-`.
Work-order frontmatter uses complete IDs for machine traceability.
Tests use isolated homes, fake commands, JSONC, manifests, and sentinel MCP fixtures.
They must run without Copilot, `harness`, Atlassian, or a signed-in developer.
Native qualification complements hermetic tests. A skipped native test does not establish runtime compatibility.

## E2E tests

| Flow | Playwright file/project | Criteria |
| --- | --- | --- |
| Select installed plugin, Save, reload, Reset, clear; native-disabled built-in cannot be selected | `tests/settings/copilot-profile-plugins.spec.ts`, `chromium` | 001.2, 002.1 through 002.4, 006.1 |
| Saved draft context with `refresh: true`; failed refresh keeps retained rows removable; missing saved selection stays visible | Same desktop file with isolated inventory fixtures | 001.4/001.6, 003.4/003.5, 006.2/006.3 |
| Phone route, selection, Save/Reset/clear, long metadata, rendered row height at least 44px, no horizontal overflow | `tests/settings/mobile-copilot-profile-plugins.spec.ts`, `mobile-chrome` | 002.4, 006.5 through 006.7 |

The settings E2E fixture intercepts only inventory responses and seeds a concrete Copilot profile through existing APIs.
The new-draft environment, duplicate, save-conflict, delayed-response, width-sweep, coarse-tablet, and deferred-preview flows are covered by component or integration tests, not by these browser specs.
No browser spec claims rendered geometry beyond the phone row height and overflow assertions above.
Backend tests separately prove native command execution, authorization, argv, executor admission, and MCP coexistence.
E2E does not use a browser-only preference or a production arbitrary-command escape hatch.
Run project commands sequentially through the guarded runner. Do not overlap full suites or override worker budgets.

## Work orders

| Order | Work order | Depends on | Status |
| --- | --- | --- | --- |
| 0 | [Task 00: Qualify native compatibility](task-00-native-compatibility.md) | None | pending |
| 1 | [Task 01: Discover and resolve installed plugins](task-01-inventory-discovery.md) | 00 | completed |
| 2 | [Task 02: Persist profile selections](task-02-profile-state.md) | 01 identity contract | completed |
| 3 | [Task 03: Prepare ACP process mounts](task-03-acp-process-loading.md) | 01, 02 | pending |
| 4 | [Task 04: Cover other profile process paths](task-04-profile-process-parity.md) | 03 | pending |
| 5 | [Task 05: Deliver the profile UI](task-05-profile-ui.md) | 01, 02, 04 preview contract | completed |
| 6 | [Task 06: Document native plugin behavior](task-06-public-docs.md) | 03, 04, 05 | pending |

Work orders are sequential. Waves express dependencies and do not authorize subagents or persistent tasks.
No generic QA, code-review, simplify, or broad verify phase is added.
Each code work order owns Red/Green tests and its exact checks.

## Verification results

Design-package checks passed on 2026-10-27:

- `python3 scripts/list-docs.py validate`: 409 decisions and 1765 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: all 36 validator tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Local PR-documentation preflight through `.github/scripts/pr-docs.cjs` `validateCoverage`: seven valid work orders, no reference errors.
  A synthetic application trigger forces graph validation because the actual diff contains Markdown only.
- All 42 acceptance criteria are assigned to work orders. Every local Markdown target exists.
- `git diff --check -- docs/specs docs/plans/copilot-profile-plugins`: passed.
  Per-file `git diff --no-index --check /dev/null <new-file>` also checked all eleven untracked Markdown files.
- Catalog lookup discovers the paired requirement and design under Agents.
- Re-ran `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `node scripts/validate-public-docs.mjs` on 2026-10-28: passed.

Implementation verification on 2026-10-27 and 2026-10-28:

- Backend build: `go build -trimpath ./...` passed.
- Package tests: `go test -trimpath -count=1` passed for `internal/agent/copilotplugins` (including real descendant-cleanup processes), `internal/agent/hostutility`, `internal/agent/settings/controller`, `internal/agent/settings/handlers`, and the settings DTO/store/MCP contract suites.
- Lifecycle tests: targeted `TestCopilotPlugin` coverage passed, including initial passthrough, resume/fallback helper, wrapper rejection, and policy rejection before PTY creation. Full package: `go test -trimpath ./internal/agent/runtime/lifecycle -count=1` passed (422.5s).
- Changed-file Go lint: `bash scripts/lint-go-changed` reported 0 issues.
- Web: `tsc --noEmit`, ESLint with `--max-warnings 0`, `i18n:check`, and `i18n:ratchet` passed.
- Vitest: twelve targeted files passed (138 tests), including saved-profile context, explicit refresh, failed-refresh retained rows, and late old-context responses.
- Desktop Playwright: 4 passed with a disposable E2E Copilot identity.
- Mobile Playwright: 1 passed with rendered row height and horizontal-overflow assertions.
- Public and internal documentation validators passed.
- Native inventory/installation and repeated mount startup were reproduced for 1.0.88 and 1.0.104.
- Authenticated plugin loading, managed-veto observation, and Windows/macOS execution remain unverified; see [compatibility-evidence.md](compatibility-evidence.md).
- Nonempty profile starts are intentionally fail-closed with `policy_unverified`. Inventory discovery, profile save/reset, and UI selection remain available. WO-00 and dependent work orders remain pending.

Fresh implementation worktrees need `(cd apps && pnpm install --frozen-lockfile)` before the first package command.
Direct Go commands include `-trimpath`. Persistence changes include SQL guard and store conformance.
Run changed-file Go lint sequentially after the applicable targeted tests.

Package completion requires all work orders to pass and every AC to retain linked evidence.
Then reconcile actual behavior before promoting draft requirements to `active`, design to `current`, and plan to `implemented`.
Do not promote records because a compatibility check was skipped or because time expired.

## Risks and unresolved evidence

- Reproduced `1.0.88` and `1.0.104` inventory, live/installed metadata, manifest checks, and repeated ACP startup. Authenticated plugin loading and managed-veto enforcement remain unverified.
- A requested npm version can advertise another binary version. Record both before claiming compatibility.
- Internal metadata and manifest precedence can change. Unsupported shapes remain visible and fail closed for selected starts.
- Explicit directory mounts may interact with managed policy differently across versions/platforms. WO-00 remains the support gate.
- Wrapper or child environment mismatches can resolve another home. Nonempty selections must fail instead of falling back.
- Native authentication was unavailable on this machine. No credentials were copied and no login was initiated.
- Omission-preserving profile updates use a storage-level selection-intent write; shared fakes without that extension retain read/modify/write snapshots.
- Remote selection rejection is a deliberate initial limitation. Empty selections keep every executor's behavior.
- E2E registration is gated by the test-only `KANDEV_E2E_MOCK_COPILOT` profile flag. The standard browser suite executes the flows; it does not require a native Copilot install or authentication.

## Public documentation impact

WO-06 added the **Load Copilot plugins for a profile** section to
`docs/public/agents-and-profiles.md`. It covers discovery, save/reset, missing
plugins, the current `policy_unverified` startup gate, local/worktree scope,
next-process timing, runtime qualification, native policy ownership, and the
deferred command preview. `README.md` and `docs/screenshots.md` had no conflicting
claim, so neither changed.
