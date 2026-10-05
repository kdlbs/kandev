---
id: "03-onboarding-form"
title: "Deliver focused onboarding agent setup"
status: complete
wave: 3
depends_on:
  - "01-opencode-arguments"
  - "02-baseline-discovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-FIRST-RUN-SETUP-001
  - REQ-AGENTS-FIRST-RUN-SETUP-002
  - REQ-AGENTS-FIRST-RUN-SETUP-003
acceptance_criteria:
  - AC-AGENTS-FIRST-RUN-SETUP-001.1
  - AC-AGENTS-FIRST-RUN-SETUP-001.2
  - AC-AGENTS-FIRST-RUN-SETUP-001.3
  - AC-AGENTS-FIRST-RUN-SETUP-001.4
  - AC-AGENTS-FIRST-RUN-SETUP-001.5
  - AC-AGENTS-FIRST-RUN-SETUP-002.1
  - AC-AGENTS-FIRST-RUN-SETUP-002.3
  - AC-AGENTS-FIRST-RUN-SETUP-002.4
  - AC-AGENTS-FIRST-RUN-SETUP-002.5
  - AC-AGENTS-FIRST-RUN-SETUP-002.6
  - AC-AGENTS-FIRST-RUN-SETUP-003.1
  - AC-AGENTS-FIRST-RUN-SETUP-003.2
  - AC-AGENTS-FIRST-RUN-SETUP-003.3
  - AC-AGENTS-FIRST-RUN-SETUP-003.4
  - AC-AGENTS-FIRST-RUN-SETUP-003.5
system_design:
  - ../../specs/agents/system-design/first-run-agent-setup.md
---

# Task 03: Deliver focused onboarding agent setup

## Summary

Give each expanded tour agent a model-only picker, adjacent refresh icon, and
supported passthrough toggle. Automatically discover its saved profile context,
save only the two edited fields, and prove the desktop/touch/phone-resize flow.

## In scope

- Write component, action-hook, and Playwright regressions before replacing the
  existing form; observe failures caused by stale startup, leaked advanced
  controls, oversized refresh, and hidden-setting writes.
- Add proposed `agent-setup-fields.tsx` and narrow `agent-settings.ts` types.
  Replace the tour's full form without adding hide flags to full settings.
- Thread the saved profile ID and complete saved launch settings into Task 02's
  baseline-only hook. Preserve saved selection labels and update row status from
  matching profile discovery, not stale global metadata.
- Model-only list selection closes the picker. No mode, dependent options,
  permissions, flags, fallback policy, or advanced disclosures are mounted.
- Derive exact model/passthrough partial patches from their saved baseline.
  Preserve hidden stored fields, skips, navigation, errors, concurrency guards,
  settings-reload interlocks, and drafts across responsive visibility changes.
- Use the executor step's bounded dialog-body scroller for the agents step and
  remove its nested list scroller. Keep navigation reachable on short screens.
- Localize help and any necessary inline empty/recovery copy in all supported
  catalogs. Keep the read-only Auto Approve warning. Update only the shipped
  tour paragraph in `docs/public/use-kandev.md` (tutorial/reference disclosure).
- Add Chromium coverage and extend the existing mobile availability spec with
  edits, resize continuity, and coarse-pointer target geometry. Run existing
  full-profile selector and capability-discovery coverage.
- Update PR capture guidance to prefer isolated repository Playwright and
  classify permission failures by capability, as explicitly requested by the user.

## Out of scope

- Phone tour enablement, permission defaults, full profile settings redesign,
  later tour steps, new APIs/schemas/flags, and live-instance mutations.

## Acceptance

- The ready tour exposes only the requested settings, loads profile models
  without a manual Refresh, and has aligned 28px desktop/44px touch controls.
  Loading/error/empty/missing-model states preserve selection and provide recovery.
- Exact save patches contain only deliberately changed model/passthrough fields;
  hidden values, failed-save drafts, skip semantics, and phone-resize continuity
  are verified through component/API/E2E evidence.
- Desktop and mobile-project regressions, full-profile continuity checks,
  localization, typecheck, focused lint, and public-doc validators pass; rendered
  screenshots match the required previews.

## ASCII UI preview

Excerpt of [the complete preview](plan.md#ascii-ui-preview), UI-02 and UI-03:

```text
Codex                                      Installed ^
Start model
[6.1 Sol                                  v] [refresh]
CLI Passthrough                              [off/on]

Loading: saved label + disabled list + busy icon.
Failure: saved label + inline explanation + refresh.
Empty:   saved label + empty explanation + refresh.

More settings: Settings > Agents.
Read-only warning about default Auto Approve remains.
[Skip]                                         [Next]
```

UI-04: Existing phone availability and restored tablet draft:

```text
Phone <768px: normal Home, no tour, no save/completion.
Return to 768px+: same model/passthrough draft restored.
Coarse pointer: selector/refresh hit targets >=44px.
```

Map UI-02/UI-03 to SETUP-001.1-.5 and SETUP-002.1/.3-.5;
UI-04 maps to SETUP-003.4. The step body is the single vertical scroll owner;
header/dots/footer stay outside. The refresh drawing names an icon action, not
a wide visible label. Whitespace/copy are illustrative, control ordering and
settings omissions are required. Inspect the nearest existing mobile availability
spec and shared control-sizing helper before implementation.

## Verification

Run from the repository root. Run E2E commands sequentially; the managed runner
rebuilds web/runtime and owns its isolated instances. No all-worker override.

```bash
(cd apps/web && pnpm exec vitest run components/onboarding-dialog.test.tsx components/onboarding/agent-setup-fields.test.tsx components/onboarding/use-onboarding-actions.test.tsx)
(cd apps/web && pnpm exec eslint components/onboarding-dialog.tsx components/onboarding/step-agents.tsx components/onboarding/agent-setup-fields.tsx components/onboarding/agent-settings.ts components/onboarding/use-onboarding-actions.ts e2e/tests/settings/onboarding-agent-setup.spec.ts e2e/tests/office/mobile-onboarding-dialog.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/onboarding-agent-setup.spec.ts tests/office/onboarding-executors.spec.ts tests/settings/profile-capability-discovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/office/mobile-onboarding-dialog.spec.ts tests/office/mobile-onboarding-dialog-rich.spec.ts tests/settings/mobile-agent-profile-config-selector.spec.ts tests/settings/mobile-profile-capability-discovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Capture/inspect desktop ready and error screenshots, a short desktop viewport,
and a coarse-pointer 768px tablet. Assert refresh width and height, selector
height, row/viewport containment, scroll ownership, missing advanced controls,
and no document horizontal overflow. Measure fine-pointer desktop controls
against 28px within 1px, not only a minimum.

## Files likely touched

- `apps/web/components/onboarding/agent-settings.ts` (new)
- `apps/web/components/onboarding/agent-setup-fields.tsx` (new)
- `apps/web/components/onboarding/agent-setup-fields.test.tsx` (new)
- `apps/web/components/onboarding/step-agents.tsx`
- `apps/web/components/onboarding/use-onboarding-actions.ts`
- `apps/web/components/onboarding/use-onboarding-actions.test.tsx` (new)
- `apps/web/components/onboarding-dialog.tsx`
- `apps/web/components/onboarding-dialog.test.tsx`
- `apps/web/e2e/tests/settings/onboarding-agent-setup.spec.ts` (new)
- `apps/web/e2e/tests/office/mobile-onboarding-dialog.spec.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-tw,zh-hk,ja,ko,pseudo}/common.json`
  and `agents.json` only where new recovery/empty labels are necessary;
  regenerate the pseudo locale with `pnpm run i18n:pseudo`.
- `docs/public/use-kandev.md`
- `.agents/skills/pr/SKILL.md`
- Scoped `apps/web/AGENTS.md` only if the extraction changes documented conventions.

## Dependencies

Tasks 01 and 02. The corrected OpenCode command supplies working discovery;
the baseline-only hook supplies the narrow form's models without hidden option work.

## Risks

- Empty saved model and saved launch env entries must not become synthetic edits
  or an empty draft launch override.
- Reopening and background agent polling must preserve same-profile edits without
  accepting late results or overwriting a successful profile status.
- Omitting a setting must omit its write path too. Exact patch and stored-field
  assertions cover Auto Approve false/true, flags, prefix, env, modes, provider
  options, and fallback values.
- The tour is intentionally unavailable on phones. Test the larger-to-phone-to-
  larger transition instead of introducing a compressed phone dialog.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/first-run-agent-setup.md)
- [Dedicated form and mutation design](../../specs/agents/system-design/first-run-agent-setup.md#dedicated-form)
- [Responsive design](../../specs/agents/system-design/first-run-agent-setup.md#responsive-composition)
- Existing `onboarding-dialog.test.tsx`, executor step scroll composition,
  `mobile-onboarding-dialog.spec.ts`, and `@kandev/ui` control sizing.

## Results

Implemented the shared onboarding form for every agent, localized help, narrow
patches, and bounded scrolling. Review regressions cover returning to a saved
model after Next/Back, edits during saves, partial failure interlocks, unchanged
completion, and empty dynamic catalogs without unrelated global choices.

The combined frontend regression run passed all 67 tests. Typecheck, focused
ESLint, all-language i18n validation, and public-document validation passed.
Playwright coverage now selects a model without Refresh, checks the exact patch
and stored hidden fields, returns to the original model, and retains a tablet
draft across phone suppression. Managed host runs passed 3 desktop and 6 mobile
tests with one shard and one worker. Isolated headless Playwright CLI verified
automatic choices without Refresh and captured ready/error, short-screen, and
coarse-pointer tablet views. Desktop refresh measured 28px; touch refresh 44px.
Empty/missing catalogs, failures, and save interlocks have focused unit coverage;
the managed E2E tests verify real profile persistence and responsive interaction.

The PR skill now directs repository captures through Playwright CLI or the
managed runner first and requires evidence from that authorized workflow before
reporting a capture blocker. Harness tests, full harness/spec lint, and targeted
pre-commit validation passed. The user requested including this skill update in
the onboarding PR.
