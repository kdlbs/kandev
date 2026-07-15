---
id: "03-general-operational-drafts"
title: "General and operational drafts"
status: done
wave: 2
depends_on: ["01-save-coordinator"]
plan: "plan.md"
spec: "../../specs/ui/settings-manual-save.md"
---

# Task 03: General and Operational Drafts

## Acceptance

- Former write-through General, Voice, Utility Agent, Feature Toggle, and changelog-notification controls remain local until Save.
- Theme preview is immediate but durable storage changes only on Save and discard restores the saved theme.
- Shortcut/feature/default resets are drafts; restart prompting follows a successful runtime-flag save.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/settings/general-settings.test.tsx components/settings/terminal-settings.test.ts components/settings/keyboard-shortcuts-card.test.tsx components/settings/voice-mode-settings.test.tsx components/settings/system-metrics-settings-card.test.tsx components/settings/system/feature-toggles-settings.test.tsx
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/theme/app-theme.tsx`
- `apps/web/components/settings/system-metrics-settings-card.tsx`
- `apps/web/components/settings/keyboard-shortcuts-card.tsx`
- `apps/web/components/settings/terminal-settings.tsx`
- `apps/web/components/settings/shell-settings-card.tsx`
- `apps/web/components/settings/voice-mode-settings.tsx`
- `apps/web/components/settings/changelog-notification-card.tsx`
- `apps/web/components/settings/utility-agents-section.tsx`
- `apps/web/components/settings/config-chat-agent-section.tsx`
- `apps/web/components/settings/system/feature-toggles-settings.tsx`

## Dependencies

Task 01.

## Inputs

- Spec: Settings-wide coverage, Immediate actions, reset/theme/feature scenarios.
- ADR 0017 and ADR 0018 for metrics/runtime flag ownership.

## Output Contract

Report each removed write-through path, composed payload ownership, preview/discard behavior, tests run, files touched, and update task/plan status.
