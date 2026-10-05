---
id: "01-skill-presentation"
title: "Preserve skill identity and improve autocomplete"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-COMMAND-AUTOCOMPLETE-001
  - REQ-AGENTS-COMMAND-AUTOCOMPLETE-002
acceptance_criteria:
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.1
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.2
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.3
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.4
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.5
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.6
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.7
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-001.8
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.1
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.2
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.3
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.4
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.5
  - AC-AGENTS-COMMAND-AUTOCOMPLETE-002.6
system_design:
  - ../../specs/agents/system-design/command-autocomplete.md
---

# Task 01: Preserve skill identity and improve autocomplete

## Summary

Implement the complete adapter-to-composer skill presentation path with regression tests.
Separate clean visible names from raw command serialization before adding the localized Skill badge.

## In scope

- Optional kind on normalized commands and matching frontend transport types.
- Positive Codex ACP classification with conservative provider fallback.
- Lifecycle cache and reconnect transport checks.
- Shared view-model derivation, clean/raw filtering, and raw-key collision handling.
- TipTap display, selection, HTML attributes, copying, draft restoration, and recall.
- Treat rich clipboard command-shaped HTML as plain visible text; keep structured command identity limited to autocomplete selection and valid draft restoration.
- Menu badge slot and complete locale entries; unit and component tests.
- Validated Codex plan action metadata, confirmed configuration projection, Mode/Active display, and argument hints.

## Out of scope

- Browser verification and public docs, owned by Task 02.
- Provider runtime changes or speculative skill discovery.

## Acceptance

1. Codex `$retro` shows `/retro` with a Skill badge; untyped and unknown provider shapes keep their established behavior.
2. Clean display names never replace raw invocation names through selection, transport, serialization, or recall.
3. Confirmed mode state and argument hints appear without configuration changes on selection; partial live provider updates preserve omitted confirmed options, and startup fences delayed prior-execution snapshots.
4. Forged rich clipboard command markup remains plain visible text and cannot alter serialized command text.

## ASCII UI preview

UI-01 and UI-02 excerpts from the [full preview](plan.md#ascii-ui-preview):

```text
Commands
  [icon] /retro [Skill]         Use when the user runs...
  [icon] /plan  [Mode] [Active] Turn plan mode off
  [icon] /goal                 Set a goal to keep pursuing
         Arguments: [<objective>|clear|pause|resume]

Composer: [ /retro ] add context |
                              [Send]
```

Desktop and phone share this order. Descriptions truncate before badges; hints stay inside the same selectable row.
Touch rows retain their existing minimum 44-pixel target. Active requires confirmed state; missing state uses a neutral description.
Required: separate raw identity and visible name. Spacing is illustrative. Applies to REQ-001 AC .1, .3, .4, .5, .6 and all REQ-002 ACs.

## Verification

Run from the repository root. Install workspace dependencies once if this checkout lacks `apps/node_modules`.

```bash
pnpm --dir apps install --frozen-lockfile
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run TestConvertAvailableCommands -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestGetAvailableCommandsForSession|TestHandleAvailableCommandsEvent' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/backendapp -run TestAppendAvailableCommandsMessage -count=1)
make -C apps/backend lint
pnpm --dir apps/web exec vitest run components/task/chat/slash-command-types.test.ts components/task/chat/tiptap-suggestion.test.ts components/task/chat/tiptap-slash-command-extension.test.ts components/task/chat/tiptap-helpers.test.ts components/task/chat/tiptap-editor-history.test.ts components/task/chat/slash-command-menu.test.tsx components/task/chat/popup-menu.test.tsx
pnpm --dir apps/web exec vitest run lib/ws/handlers/session-models.test.ts lib/ws/handlers/session-models-startup.test.ts lib/ws/handlers/session-models-user-selection.test.ts
pnpm --dir apps/web exec eslint components/task/chat/slash-command-types.ts components/task/chat/tiptap-input.tsx components/task/chat/tiptap-suggestion.tsx components/task/chat/tiptap-slash-command-utils.ts components/task/chat/tiptap-helpers.ts components/task/chat/tiptap-slash-command-extension.tsx components/task/chat/slash-command-menu.tsx components/task/chat/popup-menu.tsx lib/state/slices/session-runtime/types.ts lib/types/session-events.ts
pnpm --dir apps/web exec eslint lib/ws/handlers/session-models.ts lib/state/slices/session-runtime/session-runtime-view-actions.ts lib/state/slices/session-runtime/model-hydration.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run i18n:ratchet
git diff --check
```

Add the proposed menu test and reconnect test if they do not yet exist. Require nonzero matching test counts for each Go command.
Include any additional TS/TSX files in the targeted ESLint command if implementation changes them.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go` and `conversion_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events_routing_test.go` and `manager_interaction_accessors_test.go`
- `apps/backend/internal/backendapp/helpers_test.go` for reconnect notification coverage
- `apps/web/lib/state/slices/session-runtime/types.ts` and `apps/web/lib/types/session-events.ts`
- `apps/web/lib/ws/handlers/session-models.ts` and its three targeted test files
- `apps/web/lib/state/slices/session-runtime/session-runtime-view-actions.ts` and `model-hydration.ts` for confirmed projection propagation and invalidation
- `apps/web/components/task/chat/slash-command-types.ts` and adjacent tests
- `apps/web/components/task/chat/tiptap-input.tsx`, `tiptap-suggestion.tsx`, and adjacent tests
- `apps/web/components/task/chat/tiptap-popups.tsx` for stable suggestion-to-menu item mapping
- `apps/web/components/task/chat/tiptap-slash-command-utils.ts`, `tiptap-helpers.ts`, and adjacent tests
- `apps/web/components/task/chat/tiptap-slash-command-extension.tsx` and adjacent tests
- `apps/web/components/task/chat/slash-command-menu.tsx`, proposed `slash-command-menu.test.tsx`, and `popup-menu.tsx`
- `apps/web/src/locales/*/task.json` and generated locale artifacts

## Dependencies

None. Read the scoped backend, agentctl, and web AGENTS.md instructions before implementation.

## Risks

- Label-first serialization currently hides the difference between visible text and provider identity.
- Generic dollar stripping can affect other agents or literal names.
- The shared popup has consumers outside slash autocomplete.
- Effective configuration values can include saved preferences. Preserve a separate confirmed value projection without changing selector behavior.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/command-autocomplete.md)
- [System design](../../specs/agents/system-design/command-autocomplete.md)
- Existing conversion, composer serialization, history, and popup tests.

## Results

- Added provider-aware skill classification and validated Codex plan-action metadata. Provider names remain intact through transport and invocation while the composer displays clean labels.
- Added a separate confirmed-configuration projection, localized Skill/Mode/Active chips, and advertised argument hints. Draft selection remains local to the composer.
- Added backend, state, menu, serialization, restoration, and recall regression coverage.
- Verification passed: ACP conversion (12 matching tests plus 3 provider subtests), lifecycle cache (3 tests), and reconnect transport (1 test); backend lint reported 0 issues.
- Focused frontend tests passed: 12 files, 115 tests. Targeted ESLint, web typecheck, `i18n:check`, and `i18n:ratchet` passed.
- Review remediation carries `config_options_source` and `agent_execution_id` through the session-model WS event. Post-startup provider updates refresh confirmed raw values without a settlement marker only for the confirmed/current execution; model-only and unsettled updates retain their defined behavior. `STARTING` immediately hides prior confirmation, while matching execution identity protects reconnect ordering. The reconnect helper also retains its typed-nil manager guard.
- Remediation verification passed: backend reconnect and WS identity tests, `make -C apps/backend lint` (0 issues), frontend state/composer suite (11 files, 115 tests), web typecheck, and targeted ESLint. The managed E2E run rebuilt backend and web assets successfully.
- PR-review regressions now cover delayed settled snapshots during unresolved startup identity, preservation of omitted keys in partial `provider_update` events, a typed-nil manager, forged clipboard metadata, and additive Codex action fields. The focused regression suite passed 37 tests across 3 files; ACP and reconnect Go tests passed; backend lint reported 0 issues; web typecheck and targeted ESLint passed without warnings.
- Popup mapping memoizes the command map and current option list in `tiptap-popups.tsx`; the confirmed configuration selector uses shallow equality.
