---
id: "02-configure-image-expansion"
title: "Configure image expansion"
status: done
wave: 2
depends_on:
  - "01-expand-preview-images"
plan: "plan.md"
requirements:
  - REQ-AGENTS-IMAGE-PREVIEW-EXPANSION-001
acceptance_criteria:
  - AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.6
system_design:
  - ../../specs/agents/system-design/image-preview-expansion.md
---

# Task 02: Configure image expansion

This completed work order records the initial interpretation. Its preference
behavior is superseded by [task 03](task-03-configure-initial-disclosure.md) after
the user clarified that the setting controls initial inline disclosure.

## Summary

Add a default-on Expand image previews switch under Appearance. Store the saved
preference on this device. Use the shared settings Save and Reset controls.

## Scope

Add storage validation, UI state, a settings contributor, translated copy, and
settings discovery. Gate native file-preview activation only. Preserve inline
disclosure, loading, file-editor routing, and attachment behavior.

## Acceptance

AC .6: default enabled; unsaved changes do not persist; Reset restores the saved
value; Save persists across reloads; disabled previews remain visible and do not
open a dialog; re-enabling restores the dialog. Support desktop and phone.

## ASCII UI preview

UI-03: Settings > Preferences > Appearance, edited state. Covers AC .6.

```text
Appearance
  Color theme                  [selection]
  Animate rich-output charts   [switch]
  Chat animations              [switch]
  Expand image previews        [switch]
  Open file-preview images in a larger dialog when clicked or tapped.
  Saved on this device.

Shared save controls: [Reset] [Save changes]
```

Desktop and phone use the same row order. On phones, description text wraps
within the page and the switch has a 44px touch target. The page owns scrolling;
the existing shared save controls retain their layout. Disabled file previews
show the same inline image without a button. Ordering, labels, and interaction
are required; ASCII spacing is illustrative.

See the [combined preview](plan.md#ascii-ui-preview).

## Verification

Use TDD for default, malformed storage, hydration, and commit tests. Add a shared
browser scenario for desktop and phone to exercise the real settings controls
and file preview. Run targeted Vitest, desktop and phone browser scenarios,
TypeScript, focused ESLint, localization, specification, and public-doc checks.

## Files and exclusions

Primary files: `image-preview-expansion-settings.tsx`, `file-preview-block.tsx`,
`lib/settings/image-preview-expansion.ts`, the UI slice, appearance discovery,
and locale catalogs. Task 01 supplies the existing dialog and plain-image mode.
Exclude backend persistence, attachment preferences, and new image viewers.

## Commands

Run from the repository root. The temporary browser environment helper is
explained in task 01 and uses isolated data.

```sh
pnpm --dir apps/web exec vitest run lib/state/slices/ui/image-preview-expansion.test.ts components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx components/task/chat/messages/kandev/rich-output/rich-output-view-state.test.tsx hooks/domains/session/use-workspace-file-preview.test.ts components/task/chat/image-preview-dialog.test.tsx
pnpm --dir apps/web exec vitest run lib/settings-discovery/catalog.test.ts lib/settings-discovery/search.test.ts lib/settings-discovery/coverage-inventory.test.ts lib/settings-discovery/delivery-coverage.test.ts
NODE_OPTIONS=--max-old-space-size=6144 pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
python3 /tmp/image-expansion-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium tests/chat/image-preview-preference.spec.ts tests/chat/rich-output.spec.ts
python3 /tmp/image-expansion-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/chat/mobile-image-preview-preference.spec.ts tests/chat/mobile-rich-output.spec.ts tests/chat/mobile-message-attachment-layout.spec.ts --output e2e/test-results-image-expansion-preference
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

Implemented on 2026-10-20. The preference uses the existing Appearance page,
a dedicated save contributor, and device-local UI state. All seven languages
and the pseudo catalog include the new copy. Settings search can find the row.

- RED: five preference-state tests failed before the implementation. The desktop browser scenario failed on the absent switch. Logs: `/tmp/image-expansion-preference-red.log` and `/tmp/kandev-run.e2e.BIeAQGdp.log`.
- GREEN: 42 targeted tests passed, plus 27 existing discovery contract tests. Logs: `/tmp/image-expansion-preference-vitest.log` and `/tmp/image-expansion-preference-discovery.log`.
- Seven desktop browser tests passed. Log: `/tmp/kandev-run.e2e.4alJPUTS.log`.
- Seven phone browser tests passed, including the attachment-layout regression. Log: `/tmp/kandev-run.e2e.4gHJHags.log`.
- Typecheck and catalog drift passed with the 6 GiB heap. Log: `/tmp/image-expansion-preference-typecheck.log`.
- Focused ESLint passed for all changed TypeScript files. Log: `/tmp/image-expansion-preference-eslint.log`.
- Localization checks passed for all languages and the pseudo catalog. Log: `/tmp/image-expansion-preference-i18n-final.log`.
- Public documentation checks passed: 63 validator tests and 48 pages. Log: `/tmp/image-expansion-preference-public-docs.log`.
- Specification catalog, specification lint, documentation coverage, and whitespace checks passed.

A final regression preserves non-draggable images when expansion is disabled.
Its focused RED/GREEN evidence is in `/tmp/image-expansion-drag-red.log` and
`/tmp/image-expansion-drag-green.log`. The shared viewer tests also confirm
attachment expansion remains enabled by default.


The final focused phone scenario passed after the non-draggable-image correction
and fresh build. Log: `/tmp/kandev-run.e2e.bLI6EWdY.log`. It also checks horizontal
containment. Command:

```sh
python3 /tmp/image-expansion-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/chat/mobile-image-preview-preference.spec.ts --output e2e/test-results-image-expansion-setting-final
```

Inspected the retained `image-expansion-setting.png` under that output directory.
The label and description wrap inside the phone viewport; the switch is visible
and clear of fixed controls when scrolled into view. The result matches UI-03.
The final shared-viewer and preference tests passed seven cases. Focused ESLint
passed after the last code edit (`/tmp/image-expansion-drag-lint.log`).

AC .6 is complete. No known limitation remains within this scope. Changes remain
uncommitted.
