---
id: "03-remove-profile-runtime-panel"
title: "Remove runtime details from profile settings"
status: done
wave: 3
depends_on:
  - "02-model-runtime-recovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-003
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-003.4
  - AC-AGENTS-RUNTIME-UPDATES-003.5
  - AC-AGENTS-RUNTIME-UPDATES-003.7
system_design:
  - ../../specs/agents/system-design/runtime-model-discovery.md
---

# Task 03: Remove runtime details from profile settings

## Summary

The user rejected the profile runtime panel on 2026-10-05.
Remove the panel and its actions. Keep model controls, discovery feedback, and refresh after successful activation.
Runtime management remains in Settings > Agents > Agent runtime updates.

## Scope and owned files

- Remove runtime presentation from `apps/web/components/settings/profile-capabilities-row.tsx`.
- Keep the refresh control touch-sized below 768px and on coarse pointers in `profile-capability-status.tsx`.
- Delete `profile-runtime-info.tsx` and its component tests.
- Remove unused `profileRuntime*` keys from all `agents.json` locale catalogs.
- Update the desktop and phone profile-discovery specs without changing shared fixture semantics.
- Reconcile the owning requirements, design, plan, and public agent/profile guidance.
- Preserve backend observation metadata, the profile refresh hook, and existing runtime settings.

## Acceptance and responsive composition

1. Profile model settings show no runtime details or runtime recovery actions, including when discovery returns managed, external, or unknown metadata.
2. A successful update in another tab refreshes the current complete draft without changing its selected model or saved launch settings.
3. A failed update retains the previous catalog. Existing model refresh and authentication recovery remain available.
4. Desktop and phone share the same discovery behavior and page scroller. Phone refresh retains a 44px touch target and no horizontal overflow.

The [plan preview](plan.md#ascii-ui-preview) shows the revised desktop and phone model region.
No replacement panel, disclosure, tooltip, or update surface is introduced.

## Regression evidence

Update the existing desktop activation flow to require zero `profile-runtime-info` elements after a completed probe.
Run it against the original build and require an assertion failure with one visible panel.
After removal, run the desktop and phone profile-discovery specs.
Retain their current-draft, model-option, failed-update, and selection assertions.

## Verification

Run sequential desktop and phone E2E commands. The phone run can reuse the fresh desktop build.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/settings/use-profile-model-capabilities.test.tsx components/settings/profile-form-fields.test.tsx components/settings/profile-model-options.test.tsx)
(cd apps/web && pnpm exec eslint components/settings/profile-capabilities-row.tsx components/settings/profile-capability-status.tsx e2e/tests/settings/profile-capability-discovery.spec.ts e2e/tests/settings/mobile-profile-capability-discovery.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/profile-capability-discovery.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-profile-capability-discovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

- RED: the desktop absence assertion failed as expected against the original build. It found one runtime panel instead of zero.
- Desktop E2E passed (3 tests), and focused hook/form/model-option tests passed (36 tests).
- Mobile RED: the new narrow fine-pointer refresh check found a 36px target at 767px. The refresh control now retains a 44px target below 768px and on coarse pointers.
- GREEN: phone E2E passed all 5 tests, including the corrected narrow fine-pointer target. Desktop E2E passed 3 tests.
- Focused hook/form/model-option tests passed 36 tests after the final code change.
- Typecheck, targeted ESLint, locale completeness checks, and the new-code i18n ratchet passed.
- Public documentation tests passed 62 tests. Public docs validation, spec catalog validation, spec lint, and whitespace checks passed.
