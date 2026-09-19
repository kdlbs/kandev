---
id: task-07-profile-picker-gating
title: Block/warn profiles with gone start model in pickers
status: done
wave: 3
depends_on: [task-01-profile-fields]
plan: docs/plans/no-silent-model-fallback/plan.md
spec: docs/specs/agents/requirements/no-silent-model-fallback.md
---

# Task 7 — Profile picker gating (new-task / new-agent)

## Change

`apps/web/lib/state/slices/settings/types.ts`:

- `AgentProfileOption` gains `model`, `fallbackModel`, `autoFallback`;
  `toAgentProfileOption` populates them from the agent profile.

`apps/web/components/task-create-dialog-options.tsx` (`useAgentProfileOptions`):

- Compute gone-ness: `profile.model` non-empty and absent from the agent's
  `model_config.available_models` (look up via `availableAgents`).
- strict mode (gone, no fallback, `auto_fallback` off) →
  `disabled: true` + `disabledReason` ("start model X is no longer
  available — change it in the agent profile"). The `AgentSelector`
  (Combobox) already renders disabled options greyed with tooltips.
- fallback-model mode → selectable with an amber warning
  (reuse `getCapabilityWarning` iconography or a dedicated warning)
  "start model X is gone — fallback Y will be used".
- auto-fallback → normal.
- Profiles whose model is empty (agent default) are never gated.
- A fallback model that is itself gone does not make the profile
  selectable: it would promise a switch `SetModel` cannot apply. The
  profile is blocked like strict mode when both the start model and the
  fallback model are absent from the advertised list ("both-gone").

`apps/web/app/office/setup/agent-profile-setup-controls.tsx`
(`useSelectableProfileOptions`): same gating for office agent setup.

i18n: keys in `settings.json` — `profileStartModelUnavailable`,
`profileFallbackWillBeUsed`.

## Acceptance

1. Unit test (`useAgentProfileOptions`): strict-gone profile → disabled with
   reason; fallback-mode profile → enabled with warning; auto-fallback →
   enabled; empty-model profile → enabled.
2. Unit test (office `useSelectableProfileOptions`): same matrix.
3. The task-create dialog renders the blocked profile greyed/unselectable
   (existing Combobox disabled pattern) and the fallback warning icon.
4. i18n ratchet passes.

## Verification

```sh
cd apps/web && pnpm vitest run components/task-create-dialog-options.test.tsx app/office/setup/agent-profile-setup-controls.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:ratchet
```

## Risks

- `useExecutorProfileCompat` filters profiles for executor compatibility
  first — gating must compose with it, not replace it.
- The profile's agent model list must be loaded before gating; when
  `availableAgents` is still loading, do not mark everything disabled
  (gate only when the list is known).
