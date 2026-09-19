---
id: "03-profile-strictness-controls"
title: "Expose strictness in both profile editors"
status: done
wave: 2
depends_on: ['02-explicit-profile-policy']
plan: "plan.md"
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
acceptance_criteria:
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.6
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.7
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.10
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
---
# Task 03: Expose strictness in both profile editors

## Summary

Expose the saved profile setting through both existing editor paths.

## In scope

Expose the saved profile setting through both existing editor paths.

Update normalized/HTTP types, serialization, draft state, dirty comparison,
reconciliation, settings store options, discovery inventory, and editor submit
payloads. Add the full-width strict row above the existing fallback cards.
Preserve dormant values, disabled states, and deterministic summaries.
Update the lighter CLI profile editor and shared fallback fields too.

Use visible localized copy and existing error presentation for strict+empty
validation. Save errors retain the draft. Implement UI-01/UI-02 with shared state,
existing page scrolling, touch drawer help, focus return, and target sizing.
Translate en/pt-pt/zh-cn, generate zh-hk/zh-tw and pseudo; adjust summaries that
currently imply all auto=false profiles are strict. Keep host warnings advisory.

## Out of scope

Global policy, provider authentication, Office post-start routing redesign,
unrelated cleanup, and publication.

## Acceptance

- True, false, and omitted fields round-trip through both editors; Save/Cancel,
  reload hydration, dirty state, and partial updates preserve the intended value.
- Strictness overrides dormant settings in summaries and disabled controls;
  toggling it off restores them. Validation errors keep drafts open.
- Both editor paths satisfy UI-01/UI-02, localization, and focused component
  tests. No global setting or second mobile policy is introduced.

## ASCII UI preview

### UI-01: Desktop profile, expanded Fallback settings

Entry: Settings > Agents > profile. Default state; strictness is off.

```text
Start model                      [ saved model             v ]
Fallback settings v              Executor default allowed
+------------------------------------------------------------+
| Require exact model                              [ OFF ]   |
| Stop before starting if the selected model cannot be used.  |
| Fallback settings do not apply while this is on.            |
+-----------------------------+------------------------------+
| Automatic fallback [ OFF ]  | Explicit fallback    [ OFF ] |
| Existing help and controls  | Existing help and controls   |
+-----------------------------+------------------------------+
                                      [ Cancel ] [ Save ]
```

Strict on: summary becomes "Exact model required". Both fallback cards remain
visible and disabled, including their switches. Existing values remain saved.
Strict off restores their prior states. The fallback selector appears only when
its existing enable switch is on.
A strict+empty-model validation error appears beside the model field; Save
keeps the draft open. Collapsed state retains the summary and dirty indicator.

### UI-02: Phone profile, expanded Fallback settings

```text
< Agent profile
Start model
[ saved model                    v ]
Fallback settings v
Executor default allowed
+----------------------------------+
| Require exact model      [ OFF ] |
| Stop before starting if the      |
| selected model cannot be used.   |
| Fallback settings do not apply  |
| while this is on.                |
+----------------------------------+
| Automatic fallback       [ OFF ] |
| Visible helper text        (i)   |
+----------------------------------+
| Explicit fallback        [ OFF ] |
| Visible helper text        (i)   |
+----------------------------------+
[ Cancel ]                 [ Save ]
```

The existing page/editor scrolls as one form. Help opens the shared inset
bottom drawer and returns focus on close. Switch rows and help targets have
at least 44px touch hit areas; desktop ordinary controls keep 28px sizing.
Strict-on, collapsed, and validation states have the same semantics on phone.
Long translations wrap without horizontal overflow or hiding Save.

Order, grouping, visible disabled values, and shared state are requirements;
ASCII spacing and the illustrative model names are not pixel specifications.
UI-01/UI-02 map to AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.6 and .7.

Full context: [plan previews](plan.md#ascii-ui-preview). Rendered desktop/mobile
acceptance runs in Task 04 after this work order.

## Verification

Run from the repository root. Use TDD for changed logic; first record the
behavioral failure, then run the listed checks after implementation.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo)
(cd apps/web && pnpm exec vitest run lib/api/domains/agent-profile-normalize.test.ts components/settings/profile-form-fields.test.tsx components/settings/agent-profile-dirty.test.ts components/settings/agent-profile-reconciliation.test.ts components/agent/cli-profile-editor.test.tsx lib/settings-discovery)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint components/settings/profile-model-fields.tsx components/settings/profile-form-fields.tsx components/settings/model-fallback-settings-shell.tsx components/settings/profile-capability-helpers.tsx components/settings/agent-profile-page.tsx components/settings/agent-profile-page-state.ts components/settings/agent-profile-dirty.ts components/settings/agent-profile-reconciliation.ts components/agent/cli-profile-editor.tsx components/agent/cli-profile-fallback-fields.tsx lib/types/agent-profile.ts lib/api/domains/agent-profile-normalize.ts lib/state/slices/settings/types.ts lib/settings-discovery)
git diff --check
```

## Files likely touched

- `apps/web/lib/types/agent-profile.ts`
- `apps/web/lib/api/domains/agent-profile-normalize.ts` and tests.
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/settings-discovery/{profile-contract,coverage-inventory}.ts` and tests.
- `apps/web/components/settings/profile-{form-fields,model-fields,capability-helpers}.tsx`
- `apps/web/components/settings/model-fallback-settings-shell.tsx`
- `apps/web/components/settings/agent-profile-{page,page-state,dirty,reconciliation}.ts*`
- `apps/web/components/agent/cli-profile-{editor,fallback-fields}.tsx`
- Existing component tests and new `apps/web/components/settings/agent-profile-dirty.test.ts`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/settings.json`

## Dependencies

['02-explicit-profile-policy']; preserve Task 01 transport and candidate-lookup fixes.

## Risks

The two editor paths have separate form data; a shared shell alone does not
prove payload parity. A dormant auto=true value must not override strictness.
Record generated locale changes and avoid unrelated translation drift.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/no-silent-model-fallback.md)
- [Policy design](../../specs/agents/system-design/no-silent-model-fallback-01.md)
- [Recovery/UI design](../../specs/agents/system-design/no-silent-model-fallback-02.md)
- [Plan and regression matrix](plan.md)
- Scoped AGENTS.md, existing tests beside the affected code, and compatibility
  behavior at `ba960f973205854733e0dcd9afa355bdda3dddf6`.

## Results

Implemented on 2026-09-15. Desktop and CLI profile editors now share the
localized strictness row, preserve dormant fallback values, disable fallback
controls while strictness is enabled, and include the field in dirty state,
normalization, reconciliation, save payloads, and settings discovery. The
mobile layout keeps the existing drawer and form scroll owner.

Typecheck, i18n checks, targeted lint, and 83 focused frontend tests passed.
