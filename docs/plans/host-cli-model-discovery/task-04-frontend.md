---
id: "04-frontend"
title: "Settings and profile UI"
status: complete
wave: 4
depends_on:
  - "03-refresh-triggers"
plan: "plan.md"
requirements:
  - REQ-AGENTS-HOST-CLI-001
  - REQ-AGENTS-HOST-CLI-002
  - REQ-AGENTS-HOST-CLI-004
acceptance_criteria:
  - AC-AGENTS-HOST-CLI-001.1
  - AC-AGENTS-HOST-CLI-001.2
  - AC-AGENTS-HOST-CLI-002.2
  - AC-AGENTS-HOST-CLI-002.3
  - AC-AGENTS-HOST-CLI-004.1
  - AC-AGENTS-HOST-CLI-004.2
system_design:
  - ../../specs/agents/system-design/host-cli-model-discovery.md
---

# Task 04: Settings and profile UI

## Summary

Show the host CLI version on the agent card, and extend the shared profile
model selector with the discovery source note and the custom model ID entry.
Add copy in every shipped locale. This task adds no CLI install or update
control: that remains the existing agent-install action and the existing
managed runtime update control, both unchanged.

## In scope

- Types: `cli_version`/`cli_version_error` on `AgentDiscoveryDTO`'s frontend
  type; `ModelDiscovery`, `ModelEntry.source`, and `discovery` on
  `ModelConfig`/`DynamicModelsResponse`.
- `lib/agent-host-cli.ts`: version label and discovery-note helpers.
- `components/settings/installed-agent-card.tsx`: version line under the
  detected path (or the unknown-version reason).
- `components/settings/model-discovery-note.tsx`: source, version, and
  failure note under the model selector, with the existing retry affordance.
- `ModelConfigSelector`/`ModelConfigSelectorContent`: an `allowCustomModel`
  prop and the custom row (selects typed text verbatim while no option
  matches exactly); `ModelPicker` passes `discovery` through and renders a
  stored model absent from the list as a selectable custom entry only when
  `discovery.allows_custom_model` is true.
- Locale keys in `en`, `ja`, `pt-pt`, `zh-cn`; generated `zh-hk`, `zh-tw` via
  `pnpm run i18n:zh-hant`; `pseudo` via `pnpm run i18n:pseudo`.

## Out of scope

- Backend; Playwright specs; public docs.
- Any CLI install or update control, dialog, job, or WS wiring — none of
  that exists in this design.

## Acceptance

- The agent card renders `<DisplayName> <version>` under the detected path
  for a host-CLI agent, or a muted unknown-version line with its reason.
  Non-host-CLI agents are unchanged.
- The selector renders the custom row while typed text has no exact match,
  and selects the typed text verbatim on choose.
- A stored model absent from the discovered list renders as an enabled
  custom entry only when `allows_custom_model` is true; other agent types
  keep today's disabled "unavailable" row.
- The discovery note under the selector states the source and CLI version,
  the no-listing fallback copy for Claude, or the last discovery error with
  the existing retry action.

## ASCII UI preview

See [UI-01, UI-02 in the plan](plan.md#ascii-ui-preview). This work order
implements both views.

## Verification

```bash
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web test -- --run lib/agent-host-cli.test.ts components/model-config-selector.test.tsx)
(cd apps/web && pnpm run lint && pnpm run i18n:check)
```

## Files likely touched

- `apps/web/lib/types/http-agents.ts`
- `apps/web/lib/agent-host-cli.ts`
- `apps/web/components/settings/installed-agent-card.tsx`
- `apps/web/components/settings/model-discovery-note.tsx`
- `apps/web/components/model-config-selector.tsx`
- `apps/web/components/model-config-selector-content.tsx`
- `apps/web/src/locales/*/agents.json`

## Dependencies

Task 03.

## Risks

- The shared selector is used by chat and workflow surfaces; the custom row
  must stay opt-in (`allowCustomModel`) so those surfaces are unchanged.

## Parallelism

`sequential`

## Inputs

- Plan ASCII previews UI-01, UI-02.
- Existing `ModelPicker`, `ModelConfigSelector`, and the `openai_compatible`
  custom input precedent.

## Results

Implemented as scoped: `AgentDiscovery.cli_version`/`cli_version_error`,
`ModelDiscovery`, and `discovery` on `ModelConfig`/`DynamicModelsResponse` in
`lib/types/http-agents.ts`; `lib/agent-host-cli.ts` (version and discovery
presentation helpers); the version line in `installed-agent-card.tsx`;
`model-discovery-note.tsx`; `allowCustomModel` on
`ModelConfigSelector`/`ModelConfigSelectorContent` with the custom row;
`ModelPicker`'s `discovery` prop and the enabled-custom-entry path for a
saved model absent from the list; `useAgentCapabilities`' `discovery` field.
Locale keys added in `en`, `ja`, `pt-pt`, `zh-cn`, generated `zh-hk`/`zh-tw`
and `pseudo`. Covered by `lib/agent-host-cli.test.ts`,
`components/model-config-selector-custom-model.test.tsx`,
`components/settings/installed-agent-card.test.tsx`, and
`components/settings/profile-model-fields.test.tsx`.
