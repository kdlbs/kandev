---
id: "02-selection-recovery"
title: "Expose thinking defaults and recovery"
status: completed
wave: 2
depends_on:
  - "01-runtime-semantics"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-SETTING-SEMANTICS-001
acceptance_criteria:
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.1
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.2
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.3
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.4
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.5
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.6
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.7
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.8
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.9
system_design:
  - ../../specs/agents/system-design/session-setting-semantics.md
---

# Task 02: Selection and recovery

## Summary

Render one Thinking level control with an explicit profile default and visible
unsupported choices. Explain startup failures and route recovery to the actual source.

## In scope

- Canonical model-aware options, qualified legacy presentation, singleton default
  action, draft preservation, and explicit profile Save/clear.
- Thinking-specific recovery explanation with requested/effective values and
  source-aware navigation. Preserve existing session action ownership.
- Desktop/phone parity, translations, deterministic mock-agent E2E, and public docs.

## Out of scope

Automatic retry or prompt resend, global settings changes, upstream model metadata,
and new live-session provider-default semantics.

## Acceptance

1. Saving Use provider default clears both aliases, preserves other settings,
   survives reload, and admits a fresh task with the exact selected model.
2. Invalid saved values remain visible until explicit replacement/clear. A failed
   or stale capability probe cannot erase drafts. Recovery identifies the winning source.
3. Phone and desktop complete the same flow with localized copy and existing
   primitives. Existing permission selectors retain their behavior.

## ASCII UI preview

UI-01: Profile editor, unsupported saved thinking level (proposed).

```text
Desktop: Settings > Agents > Pi > Planning
Model             [ Argon High                         v ]
Thinking level    [ medium (unavailable)                v ]
This model supports only off. Saved choice: medium.
Choices: Use provider default | off
                                  [Discard] [Save]

Phone: the same profile page opens one choice sheet
[Back]                 Thinking level             [Close]
( ) Use provider default
( ) off
Saved: medium (unavailable)
Select a choice to update the draft.
```

The phone sheet owns one vertical scroller. Header and choice rows remain reachable
above safe areas; profile Save remains the persistence action. Targets are at
least 44px on phone/coarse pointers, while desktop uses 28px shared controls.
Changing viewport retains the draft. This reuses `model-config-sheet.tsx` and
the `MobilePickerSheet` interaction pattern, not a new nested overlay.

UI-02: Startup recovery card, profile-owned choice (proposed, same hierarchy on phone).

```text
Thinking level is unavailable
Planning requests medium. Argon High supports only off.
No prompt was sent.
[Edit profile]
Choose Use provider default and save before starting a fresh session.
```

A session/workflow source names that source and its existing edit/reset route.
Existing retry and fresh-session controls retain their ownership and guards.
The card must not imply Edit profile clears a stronger override or sends a prompt.
Unavailable copy, source visibility, default selection, control order, and
single-sheet phone behavior are structural requirements. Spacing is illustrative.
Map both views to AC-AGENTS-SESSION-SETTING-SEMANTICS-001.4, .6, and .9.

The full combined preview and evidence are in [the plan](plan.md#ascii-ui-preview).

## Verification

Install workspace dependencies once from `apps/` with `pnpm install --frozen-lockfile`
if this worktree has no install. Run these commands from repository root:

```bash
(cd apps/web && pnpm test components/settings/profile-form-fields.test.tsx components/settings/profile-model-config.test.ts hooks/domains/settings/use-profile-model-capabilities.test.tsx components/task/chat/session-bootstrap-recovery-model.test.ts components/task/chat/session-bootstrap-recovery-card.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/pi-thinking-settings.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-pi-thinking-settings.spec.ts tests/task/mobile-session-mode-mismatch.spec.ts)
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

E2E files are planned outputs. Run suites sequentially with the managed runner.
Add any new standalone unit test files to the exact command before completion.
Re-run Task 01's targeted tests after final integration. Record actual results,
not the diagnostic smoke-test results from before implementation.

## Files likely touched

- `apps/web/components/settings/profile-form-fields.tsx`, `profile-model-fields.tsx`,
  `profile-model-config.ts`, and `mode-combobox.tsx` plus tests.
- `apps/web/hooks/domains/settings/use-profile-model-capabilities.ts`,
  `use-profile-model-options.ts`, and corresponding tests.
- `apps/web/components/model-config-selector.tsx`, selector rows/content, and `model-config-sheet.tsx`.
- `apps/web/components/task/mode-selector.tsx` and session runtime projection/WS handlers.
- `apps/web/components/task/chat/session-bootstrap-recovery-model.ts`,
  `session-bootstrap-cause-labels.ts`, recovery content/card, and tests.
- `apps/web/src/locales/` required catalogs and generated Traditional Chinese/pseudo catalogs.
- `apps/backend/cmd/mock-agent/` minimal Pi-shaped capability and mode fixtures.
- `apps/web/e2e/tests/settings/pi-thinking-settings.spec.ts` and mobile counterpart (new).
- `docs/public/agents-and-profiles.md`: explain Thinking level, default clearing,
  override precedence, and the distinction between startup success and provider output.

## Dependencies

Task 01. Follow current frontend and mock-agent scoped AGENTS.md guidance.

## Risks

Do not hide the default action when only off is supported. Avoid silently dropping
invalid values in option reconciliation or replacing a session override with a
profile edit. Choice selection changes the draft; profile Save persists it.

## Parallelism

`sequential`

## Inputs

[Requirements](../../specs/agents/requirements/session-setting-semantics.md),
[design](../../specs/agents/system-design/session-setting-semantics.md), and Task 01's contracts.
Use `model-config-sheet.tsx` and `mobile-session-mode-mismatch.spec.ts` as precedents.
Apply better-ui and mobile-parity without introducing new decorative surfaces.

## Results

Completed on 2026-11-19.

- Added one Pi Thinking level field with Use provider default, explicit off, and visible unavailable saved intent. Modern options suppress the duplicate legacy control; legacy-only Pi keeps a correctly labelled selector.
- Added profile-owned recovery guidance to both the legacy bootstrap card and the current composer recovery card. The browser regression exposed the second rendering path. Stronger override evidence remains distinct from profile-owned intent.
- Added the thinking cause codes to existing recovery occurrence matching so an unrelated successful recovery cannot settle a thinking failure.
- Updated all seven production locale catalogs and generated pseudo/Traditional Chinese entries. Updated the public profiles guide.
- 230 tests in 12 focused frontend files passed. Web typecheck, lint, full localization checks, and the new-code localization ratchet passed.
- Desktop production-browser flow passed: unsupported startup, thinking-specific cause and no-prompt message, reload, Edit profile, default Save/reload, exact model retained, and a fresh task with an actual mock assistant response.
- Phone production-browser flow passed: the same recovery route, one thinking sheet, 44px targets, close/focus return, unavailable/default selection, Save/reload, and no horizontal overflow. The existing phone permission-mode mismatch test passed in the same worker after this test. Screenshot inspected: thinking field and default value fit the phone layout.
- Failure-card fixtures disable automatic resume through the existing user preference and restore its previous value. This keeps reload verification separate from the already-supported provider-restored recovery path. The disposable Pi registration is removed after each test.
- Public documentation validation and 63 validator tests passed. Specification catalog/lint and whitespace checks passed.

PR review added backend failure assertions before browser recovery checks, made saved thinking choices visible when the option ID differs, and separated recovery guidance by setting source. A thinking option remains available when its ID is `mode`. The four focused selector and recovery files passed.

### Final focused frontend command

```bash
cd apps/web
pnpm test components/settings/profile-thinking.test.ts components/task/selector-trigger-class.test.tsx components/task/chat/session-bootstrap-recovery-model.test.ts components/settings/profile-form-fields.test.tsx components/settings/profile-model-config.test.ts hooks/domains/settings/use-profile-model-capabilities.test.tsx hooks/domains/settings/pi-thinking-capabilities.test.tsx components/task/chat/session-bootstrap-recovery-card.test.tsx components/task/chat/session-recovery-card.test.tsx lib/session-last-agent-error.test.ts hooks/processed-message-filtering.test.ts lib/active-session-recovery.test.ts
```

### Local browser environment

The managed runner required removal of inherited `KANDEV_INTERNAL_CONFIG_FILE`,
`KANDEV_INTERNAL_CONFIG_HOME_FILE`, and `KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG`.
Chromium also required the installed Nix shared-library directories in
`LD_LIBRARY_PATH`. These were per-process test settings; no host configuration
was edited. Initial harness failures, an outdated artifact guard, and the
recovery-card wiring failure were diagnosed before the final passing runs.
