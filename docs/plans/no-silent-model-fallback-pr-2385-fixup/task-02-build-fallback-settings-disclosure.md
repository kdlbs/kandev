---
id: "02-build-fallback-settings-disclosure"
title: "Build fallback settings disclosure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/no-silent-model-fallback.md"
---

# Task 02: Build fallback settings disclosure

## Intent

Group the two profile fallback strategies in a compact, self-describing
collapsible section that presents them side by side on desktop, intentionally
stacks them on phones, and gives every help icon a keyboard and touch path.

## Acceptance

1. Both full and inline profile editors start with Fallback settings collapsed;
   the header summarizes strict, automatic, or explicit-fallback mode and
   exposes hidden dirty state.
2. Expanded content renders two equal columns at `md` and one column below it.
   Automatic fallback disables, but does not remove or clear, the explicit
   fallback controls.
3. Each option retains visible helper copy. Its info button opens localized
   help on hover/focus for fine pointers and in a `useTouchDrawer` bottom drawer
   for coarse pointers, with at least a 44px touch target.

## Mobile Design Contract

- **Entry point and outcome:** desktop and mobile use the existing agent-profile
  form; both can inspect and configure either fallback strategy.
- **Nearest exemplars:** `ExternalMcpSettings.ToolsPreview` supplies the
  semantic collapsible/chevron pattern, and `SleepInhibitionInfoTooltip`
  supplies fine-pointer Tooltip versus coarse-pointer Drawer behavior.
- **Hierarchy:** closed header first reports the effective strategy; expanded
  cards place their label/switch first, visible helper second, and optional
  model picker within the explicit card.
- **Surface:** inline disclosure for the persistent settings; inset bottom
  drawer only for short, temporary help on touch.
- **Geometry and scroll:** `md:grid-cols-2`, one column below `md`, normal page
  scrolling, 44px disclosure/help targets on touch, no horizontal document
  overflow, and shared form state across viewports.
- **Primary action:** opening the disclosure and toggling a strategy remain
  direct semantic button/switch actions; no hover-only or long-press behavior.

## TDD Sequence

1. Add shell/caller tests for initial collapse, summaries, expansion, dirty
   decoration, mutual exclusion, and desktop help focus; confirm they fail.
2. Add the shared shell and help primitive, reusing `Collapsible`, `Tooltip`,
   `Drawer`, and `useTouchDrawer`.
3. Adapt the full settings and inline CLI profile callers without changing
   persistence callbacks or model-picker behavior.
4. Add all localized copy and run focused component, i18n, and type checks.

## Files Likely Touched

- `apps/web/components/settings/model-fallback-settings-shell.tsx` (new)
- `apps/web/components/settings/model-fallback-settings-shell.test.tsx` (new)
- `apps/web/components/settings/profile-model-fields.tsx`
- `apps/web/components/settings/profile-form-fields.test.tsx`
- `apps/web/components/agent/cli-profile-fallback-fields.tsx`
- `apps/web/components/agent/cli-profile-editor.test.tsx`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pseudo/settings.json`
- `apps/web/src/locales/pt-pt/settings.json`
- `apps/web/src/locales/zh-cn/settings.json`

## Dependencies

None.

## Parallelism

`parallel-safe` with Task 01; the files are disjoint and neither task changes a
shared contract or package configuration.

## Inputs

- Spec section “Profile editor rows”.
- `apps/web/components/settings/external-mcp-settings.tsx` collapsible pattern.
- `apps/web/components/settings/sleep-inhibition-settings.tsx` input-modality
  help pattern.
- Current `ModelFallbackSection` and `ModelFallbackFields` form-state callbacks.

## Verification

```sh
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- components/settings/model-fallback-settings-shell.test.tsx components/settings/profile-form-fields.test.tsx components/agent/cli-profile-editor.test.tsx
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
cd apps/web && pnpm run typecheck
```

## Risks

- Preserve saved explicit fallback values while automatic mode has precedence;
  disabling the card must not emit an `onChange` patch.
- Keep existing `profile-auto-fallback-field` and
  `profile-fallback-model-field` test IDs while adding section/help IDs.
- Keep visible helper copy; the new tooltip/drawer cannot become the only
  explanation.

## Output Contract

Report component boundaries, desktop/mobile behavior, localized keys, exact
commands and outcomes, files changed, risks, and synchronized task/plan status.

## Results

Implemented the shared `ModelFallbackSettingsShell` and reused it in both the
full agent profile form and the inline CLI profile editor. The disclosure
starts collapsed with strict/automatic/explicit summaries, expands to a
two-column `md` grid and stacks below `md`, and keeps explicit fallback values
visible but disabled while automatic fallback is enabled. Each option retains
visible helper text and has localized fine-pointer tooltip and coarse-pointer
drawer help with 44px touch targets.

Added localized keys to `en`, `pseudo`, `pt-pt`, and `zh-cn`, while keeping the
existing fallback field test IDs and save callbacks intact.

Verification:

- `cd apps && pnpm --filter @kandev/web test -- components/agent/cli-profile-editor.test.tsx components/settings/model-fallback-settings-shell.test.tsx components/settings/profile-form-fields.test.tsx` — passed (21 tests).
- `cd apps/web && pnpm run i18n:check` — passed; reported the repository's existing advisory parity gaps in `pt-pt`/`zh-cn`.
- `cd apps/web && pnpm run i18n:ratchet` — passed.
- `cd apps/web && pnpm run typecheck` — passed.
- Targeted ESLint and Prettier checks for changed web files — passed.

Files changed: shared shell/test, full and inline callers, profile form test,
four settings catalogs.

Risk retained: explicit fallback values are not cleared when automatic mode
takes precedence; the disabled helper explains how to edit them again.
