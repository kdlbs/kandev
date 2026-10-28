---
id: "05-profile-ui"
title: "Select Copilot plugins in desktop and phone profile settings"
status: completed
wave: 6
depends_on: ["01-inventory-discovery", "02-profile-state", "04-profile-process-parity"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-002
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-001.2
  - AC-AGENTS-COPILOT-PLUGINS-001.4
  - AC-AGENTS-COPILOT-PLUGINS-001.6
  - AC-AGENTS-COPILOT-PLUGINS-002.3
  - AC-AGENTS-COPILOT-PLUGINS-002.4
  - AC-AGENTS-COPILOT-PLUGINS-003.4
  - AC-AGENTS-COPILOT-PLUGINS-003.5
  - AC-AGENTS-COPILOT-PLUGINS-004.6
  - AC-AGENTS-COPILOT-PLUGINS-006.1
  - AC-AGENTS-COPILOT-PLUGINS-006.2
  - AC-AGENTS-COPILOT-PLUGINS-006.3
  - AC-AGENTS-COPILOT-PLUGINS-006.4
  - AC-AGENTS-COPILOT-PLUGINS-006.5
  - AC-AGENTS-COPILOT-PLUGINS-006.6
  - AC-AGENTS-COPILOT-PLUGINS-006.7
  - AC-AGENTS-COPILOT-PLUGINS-007.4
  - AC-AGENTS-COPILOT-PLUGINS-007.5
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
---

# Task 05: Deliver the profile UI

## Summary

Deliver inline Copilot plugin selection using the existing profile save flow.
Prove the same editing outcome on desktop and phone with hermetic inventory fixtures.

## In scope

- Add the dedicated Copilot selector, small selection-row derivation helper, safe discovery types, action/API client, and domain hook.
- Render on concrete Copilot saved profiles and the existing concrete profile creation/edit flow.
- Show metadata, native status, Refresh, supported/unsupported availability, true empty, loading, stale context, failed discovery, and missing saved choices.
- Preserve missing choices as removable rows. Failed refresh never clears or saves a draft.
- Pass complete draft launch context on explicit Refresh. Cancel and fence profile/draft/runtime/request races and StrictMode remounts.
- Use existing dirty/save/Reset/conflict state from WO-02. Do not add a local save bar or duplicate contributor.
- Render the deferred-plugin note beside the base command preview without exposing roots or copying misleading executable placeholders.
- Add seven-language copy. Generate Traditional Chinese and pseudo through repository scripts.
- Add desktop/mobile E2E flows from the plan and the nearest Cursor selection exemplar.

## Out of scope

Cursor behavior changes, native commands from React, plugin management, remote controls, a generic selection framework, or new settings navigation.

## Acceptance

1. Desktop and new-profile flows select native-disabled installed plugins and preserve Save/reload/duplicate/Reset/clear/conflict behavior.
2. Failed/empty/stale/missing/unsupported states stay distinct, allow removal, reject late responses, and explain native whole-plugin effects.
3. Phone and coarse-pointer controls meet measured 44px targets, labels wrap, settings keeps one scroller, and translated views have no horizontal overflow.

## ASCII UI preview

The combined [plan preview](plan.md#ascii-ui-preview) is authoritative for UI-01 through UI-04.
These excerpts retain the same view labels. Spacing and English wording are illustrative.
The inline hierarchy, shared save flow, state distinctions, and wrapping are structural.

### UI-01: Desktop profile, ready and dirty

```text
+----------------------------------------------------------------+
| Profile Settings                                               |
| +------------------------------------------------------------+ |
| | Plugins *                                      [Refresh] | |
| | Load whole plugins. They can run tools, skills and hooks.   | |
| | [x] harness                                                | |
| |     gst-agentic-platform  1.2.0  Native status: disabled     | |
| | [ ] browser-use                              Unavailable   | |
| |     Built-in. Cannot load by directory.                     | |
| | Local/worktree only. Applies to new processes.              | |
| | Copilot may load other native plugins.                     | |
| +------------------------------------------------------------+ |
| Base command excludes mounts resolved at process start.        |
|                 Unsaved changes       [Reset] [Save]            |
+----------------------------------------------------------------+
```

### UI-02: Phone profile, ready and dirty

```text
+--------------------------------------+
| Settings > Copilot > Work profile     |
+--------------------------------------+
| Profile Settings                     |
| +----------------------------------+ |
| | Plugins *                        | |
| | [Refresh]                        | |
| | Load whole plugins, including    | |
| | executable tools and hooks.      | |
| | [x] harness                      | |
| |     gst-agentic-platform         | |
| |     1.2.0                        | |
| |     Native status: disabled      | |
| | [ ] browser-use                  | |
| |     Unavailable. Built-in.       | |
| | Local/worktree only. New         | |
| | processes use saved selections.  | |
| | Copilot may load other plugins.  | |
| +----------------------------------+ |
| Unsaved changes    [Reset] [Save]     |
|          existing safe-area gap      |
+--------------------------------------+
```

Use the existing focused phone settings route and `settings-scroll-container`.
Do not add a drawer or nested plugin scroller. Preserve floating save clearance and safe-area handling.
Phone composition applies below 768px even with a fine pointer.
Targets measure at least 44px on phones/coarse pointers. Desktop Refresh remains 28px.

### UI-03: Inventory and repair states

```text
Loading:     Loading plugins... Saved choices stay shown.
Empty:       No installed plugins found. Install with Copilot, then refresh.
Failed:      Could not refresh. Saved choices are unchanged. [Refresh]
             [x] harness (saved, availability not checked)
Missing:     [x] harness (saved, unavailable)
             Repair its install or uncheck it before a new process starts.
Stale:       Launch settings changed. [Refresh] to check this context.
Unsupported: This runtime does not support profile plugin loading.
```

Unselected unavailable rows cannot be checked. Selected unavailable rows can always be unchecked.
Refresh keeps its accessible name, with progress announced in a translated status region.

### UI-04: Existing external conflict flow

```text
This profile changed elsewhere. [existing recovery actions]
Plugins keeps the local draft while the saved baseline updates.
```

Mappings: AC-...-002.4, AC-...-003.4/003.5, AC-...-004.6, AC-...-006.1 through 006.7.
The full prefix is `AC-AGENTS-COPILOT-PLUGINS`.

## Verification

Use `/tdd`, `/mobile-parity`, and `/e2e`. Run from the repository root.
New files named below are implementation outputs. Create their meaningful tests before logic changes.
Workspace dependencies must already exist from WO-02.

```bash
(cd apps/web && pnpm exec vitest run components/settings/copilot-plugin-selection.test.ts components/settings/copilot-plugin-profile-selection.test.tsx hooks/domains/settings/use-copilot-plugin-discovery.test.tsx components/settings/agent-profile-page.test.tsx components/settings/profile-form-fields.test.tsx 'app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.test.tsx')
(cd apps/web && pnpm exec eslint --max-warnings 0 components/settings/copilot-plugin-profile-selection.tsx components/settings/copilot-plugin-selection.ts hooks/domains/settings/use-copilot-plugin-discovery.ts components/settings/agent-profile-page.tsx components/settings/profile-form-fields.tsx 'app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.tsx')
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host -- --project=chromium tests/settings/copilot-profile-plugins.spec.ts)
(cd apps/web && pnpm e2e:run --host -- --project=mobile-chrome tests/settings/mobile-copilot-profile-plugins.spec.ts)
git diff --check
```

Before the i18n checks, run `(cd apps/web && pnpm run i18n:zh-hant)` and `(cd apps/web && pnpm run i18n:pseudo)`.
Record locale changes and any verbatim declarations with reasons.
Extend the ESLint list for every additional modified source file.
Run E2E projects sequentially. Never override worker budgets or overlap full suites.

Rendered checks include the Pixel-5 project, 767px and 768px boundaries, a narrow fine pointer, and a coarse tablet.
Assert actual hit areas, row/control containment, wrapping, scroll ownership, and no document horizontal overflow.
Inspect a captured phone view against UI-02. Assert keyboard checkbox/Refresh access and visible reasons without hover.
Prove desktop-to-phone-to-desktop keeps an unsaved selection.
For failed-refresh races, use causal route gates rather than wall-clock sleeps.

## Files likely touched

- `apps/web/components/settings/copilot-plugin-profile-selection.tsx`, `copilot-plugin-selection.ts`, and focused tests (new).
- `apps/web/hooks/domains/settings/use-copilot-plugin-discovery.ts` and `.test.tsx` (new).
- `apps/web/lib/types/copilot-plugin-discovery.ts` (new), existing agent action/API client modules.
- `apps/web/components/settings/{agent-profile-page.tsx,profile-form-fields.tsx}` and their tests.
- Existing agent-level concrete profile creation/edit flow and fixtures.
- `apps/web/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.tsx` and `.test.tsx`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/agents.json` and applicable preview namespace catalogs.
- `apps/web/e2e/tests/settings/{copilot-profile-plugins.spec.ts,mobile-copilot-profile-plugins.spec.ts}` and a disposable fixture helper (new).

## Dependencies

WO-01 provides inventory and safe metadata. WO-02 provides complete state/save semantics.
WO-04 fixes the deferred-preview contract and profile probe request shape.

## Risks

Selection and native status must not share the same checkbox meaning.
Unavailable selected checkboxes must stay removable during loading, failures, and runtime changes.
New draft IDs cannot be sent as persisted profile identities.
Existing shared settings geometry remains authoritative. Do not change unrelated navigation or floating-save layout.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/copilot-profile-plugins.md#editor-state), composition and preview sections.
- [Full previews and flow matrix](plan.md#ascii-ui-preview).
- `CursorMcpProfileSelection`, `mobile-cursor-plugin-mcp.spec.ts`, `SettingsLayoutClient`, and `SettingsFloatingSave`.
- `.agents/skills/mobile-parity/references/kandev-mobile-ui-language.md`, control sizing, and `docs/i18n.md`.

## Results

Completed 2026-10-28 for the UI scope. Nonempty process starts remain gated by WO-00 and are not part of this work order's acceptance.

- `pnpm exec vitest run` for twelve selector, helper, hook, preview, dirty, reconciliation, page-state, save-helper, duplicate, and profile-page files: 138 tests passed after the saved-draft, refresh, failed-refresh, and late-response regressions.
- `pnpm exec eslint --max-warnings 0` for the modified source list: passed.
- `pnpm exec tsc --noEmit`: passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`: passed after adding seven catalogs, generated Traditional Chinese, pseudo, and a pt-pt verbatim declaration for "Plugins".
- `KANDEV_E2E_MOCK_COPILOT=true` in the `e2e` profile registers a disposable Copilot settings identity. Discovery routes are intercepted, so the flows need no native Copilot install or authentication.
- `pnpm e2e:run --host -- --project=chromium tests/settings/copilot-profile-plugins.spec.ts`: 4 passed. Covers select/save/reload/Reset/clear, saved draft context plus `refresh: true`, failed refresh with removable retained rows, and a missing saved plugin.
- `pnpm e2e:run --host -- --project=mobile-chrome tests/settings/mobile-copilot-profile-plugins.spec.ts`: 1 passed. Asserts a rendered row height of at least 44px and no horizontal overflow with a long plugin name at 390px, preserves the selection at 767px, and covers save/Reset/clear on a phone.
- The selector derives row availability from the current status and stale context, so a retained response after a failed refresh is removable but labeled availability-unchecked and cannot add plugins.
