---
id: "01-hide-setting-hook-and-toggle"
title: "Hide-setting hook and settings-page toggle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/hide-disabled-profiles-nav.md"
---

# Task 01: Hide-setting hook and settings-page toggle

## Acceptance

- `useHideDisabledAgentProfilesInNav()` returns `{ hideDisabled, setHideDisabled }`; `hideDisabled` defaults to `false`, is `true` only when `localStorage["kandev:agents:hideDisabledInNav:v1"] === "true"`, persists via `setHideDisabled`, reacts to the `kandev:agents:hide-disabled-in-nav-changed` custom event and the `storage` event, degrades to `false` on a read error, and throws on a failed write.
- `/settings/agents` renders a "Hide disabled agent profiles from left panel navigation" row (`Switch id="hide-disabled-agent-profiles-in-nav"`) that reflects the hook value and toggles it immediately (no save bar).
- The two new locale keys exist in all four catalogs (`en`, `pseudo`, `pt-pt`, `zh-cn`) and pass `i18n:ratchet` / `i18n:check`.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- \
  hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav.test.ts \
  app/settings/agents/hide-disabled-agent-profiles-setting.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:ratchet && pnpm run i18n:check
```

## Files likely touched

- `apps/web/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav.ts` (new)
- `apps/web/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav.test.ts` (new)
- `apps/web/hooks/domains/integrations/local-storage-mock.test-helpers.ts` (reuse as-is)
- `apps/web/app/settings/agents/hide-disabled-agent-profiles-setting.tsx` (new)
- `apps/web/app/settings/agents/hide-disabled-agent-profiles-setting.test.tsx` (new)
- `apps/web/app/settings/agents/page.tsx` (render the new row inside
  `AgentProfilesSection`, below the "Agent Profiles" header and above the
  first profile row)
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/settings.json` (two keys each)

## Dependencies

None.

## Parallelism

Sequential. Tasks 02 and 03 consume the hook contract this task defines.

## Inputs

- Spec: `What`, `Data model`, `API surface`, `Failure modes`,
  `Persistence guarantees`, and the toggle scenarios.
- Plan: `Frontend > 1. Hide-setting hook` and `Frontend > 2. Settings-page toggle`.
- Existing patterns: `apps/web/hooks/domains/integrations/use-hide-disabled-integrations-in-nav.ts`
  (+ its test), `apps/web/components/integrations/integrations-index-page.tsx`
  (the `HideDisabledIntegrationsSetting` row), `apps/web/app/settings/agents/profile-list-item.tsx`
  (domain-local component placement), `apps/web/hooks/domains/settings/use-profile-enabled-toggle.ts`
  (immediate-save page convention).

## Risks

- The page is on the whole-file i18n guard list: the new row's copy must use
  `t()` and the exact `settings:` keys above; any literal breaks
  `i18n:ratchet`.
- The new component file lives under `app/settings/agents/**` — the guard
  list covers it, so keep all user-facing strings in `t()`.
- `Switch` `checked` must come from the hook snapshot, and
  `onCheckedChange` must call `setHideDisabled(next)` directly — no local
  state that can desync from the tree filter in task 02.
- No em dash (U+2014) in any copy or locale value.

## Output contract

Report the hook behavior, files changed, exact commands and results,
blockers/risks, then mark this task `done` and update its checkbox in
`plan.md`.

## Completion report

- Hook: `useHideDisabledAgentProfilesInNav` matches the integrations hook
  shape; storage key `kandev:agents:hideDisabledInNav:v1`, sync event
  `kandev:agents:hide-disabled-in-nav-changed`; degrades to `false` on read
  errors, throws on failed writes.
- Toggle: `HideDisabledAgentProfilesSetting` rendered inside
  `AgentProfilesSection` on `/settings/agents`, below the "Agent Profiles"
  header and above the first profile row; immediate-save switch
  `#hide-disabled-agent-profiles-in-nav`.
- Locales: two keys added to `en`, `pt-pt`, `zh-cn`; `pseudo` regenerated.
- Focused tests: hook 10/10, toggle 3/3; `i18n:ratchet` + `i18n:check`
  clean; typecheck clean.
- Blockers: none.
