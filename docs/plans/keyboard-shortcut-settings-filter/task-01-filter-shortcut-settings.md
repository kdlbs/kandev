---
id: "01-filter-shortcut-settings"
title: "Filter keyboard shortcut settings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SHORTCUT-SETTINGS-FILTER-001
acceptance_criteria:
  - AC-UI-SHORTCUT-SETTINGS-FILTER-001.1
  - AC-UI-SHORTCUT-SETTINGS-FILTER-001.2
  - AC-UI-SHORTCUT-SETTINGS-FILTER-001.3
  - AC-UI-SHORTCUT-SETTINGS-FILTER-001.4
  - AC-UI-SHORTCUT-SETTINGS-FILTER-001.5
system_design:
  - ../../specs/ui/system-design/keyboard-shortcut-settings-filter.md
---

# Task 01: Filter keyboard shortcut settings

## Summary

Add local search to the existing general keyboard shortcut card while preserving
its route draft and full-set conflict detection. Prove the actual card and both
real settings-route viewport flows, and update the existing public how-to.

## In scope

- Use translated labels, trimmed case-insensitive substring matching, stable
  group order, a single no-match message and an accessible clear action.
- Keep complete conflict inputs and existing draft, recorder, reset, clear and
  save paths. Cover dynamic integration labels and hidden plugin peers.
- Add focused component and desktop/phone browser tests, screenshots and docs.

## Out of scope

Key-chord search, plugin-settings filtering, backend/storage/API/save changes,
recorder touch redesign, broad layout and settings-load work.

## Acceptance

1. Search and clear affect visible rows only; blank restores the original list,
   hidden draft edits persist, and visible conflict warnings include hidden peers.
2. The inline control and empty state match UI-01 to UI-03 on both viewports;
   accessible names, target dimensions, scroll owner, and overflow meet AC .5.
3. Actual component RED precedes implementation; targeted tests, visual evidence,
   and public documentation are completed and recorded under the ROOT release.

## ASCII UI preview

[Full preview and structural notes](plan.md#ascii-ui-preview); AC .1-.5.

UI-01 desktop, matching query:

```text
Keyboard Shortcuts
[ Search shortcuts: github                 ] [clear]
Integrations
Open GitHub                       [ Unbound ]
```

UI-02 phone, matching query, in the existing settings scroller:

```text
Keyboard Shortcuts
[ Search shortcuts: github ] [clear]
Integrations
Open GitHub
[ Unbound ]
```

UI-03 both viewports, no matches:

```text
[ Search shortcuts: unmatched ] [clear]
No matching shortcuts
```

## Verification

Run only after ROOT's separate implementation and HEAVY release. Read `/tdd`,
`/e2e`, `/mobile-parity`, and `/docs-maintainer` at execution. Fresh worktree:
install once from `apps` before any pnpm command or hooks. Pin tools as below.
Run each command sequentially and retain each native process handle, actual join,
exit status and log. `quiet` allocates a unique external directory per invocation;
`--summary` belongs only to E2E wrapper invocations. Do not run full suites.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export GOTOOLCHAIN=local
quiet() { KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-shortcut-filter-quiet.XXXXXXXX)" scripts/run-quiet "$@"; }
quiet install -- pnpm --dir apps install --frozen-lockfile
```

Add the mounted-card test first and run the following once, recording its actual
failure attributable to the missing filter. Then implement and run it again:

```bash
quiet web-test -- pnpm --dir apps/web exec vitest run components/settings/keyboard-shortcuts-card.test.tsx
```

If a new helper test is added, include its exact path in an additional focused
Vitest invocation and record it. Then run these targeted checks:

```bash
quiet web-lint -- pnpm --dir apps/web exec eslint --max-warnings 0 components/settings/keyboard-shortcuts-card.tsx components/settings/keyboard-shortcuts-card.test.tsx e2e/tests/settings/shortcut-filter-helpers.ts e2e/tests/settings/keyboard-shortcuts-filter.spec.ts e2e/tests/settings/mobile-keyboard-shortcuts-filter.spec.ts
quiet typecheck -- env NODE_OPTIONS=--max-old-space-size=8192 pnpm --dir apps/web run typecheck
quiet i18n -- pnpm --dir apps/web run i18n:check
quiet i18n-ratchet -- pnpm --dir apps/web run i18n:ratchet
quiet docs-test -- node --test scripts/validate-public-docs.test.mjs
quiet docs -- node scripts/validate-public-docs.mjs
quiet spec-catalog -- python3 scripts/list-docs.py validate
quiet spec-lint-tests -- python3 scripts/lint-spec-files.test.py
quiet spec-lint -- python3 scripts/lint-spec-files.py --all
quiet diff -- git diff --check
```

If a helper is added, include it and its test in targeted lint. Do not install
again or run hooks until authorized delivery. Retain successful matching build
receipts at the same source state before either `--no-build` E2E invocation;
rerun an affected build if its inputs change. These mirror the host runner's
backend, delivery fixture, web and plugin package build steps:

```bash
quiet build-backend -- make -C apps/backend build build-e2e-delivery-fixture
quiet build-web -- make build-web-e2e
quiet build-plugin -- make -C apps/backend e2e-plugin-ui e2e-plugin-package
quiet e2e --summary -- env CAPTURE_PR_ASSETS=true pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project chromium tests/settings/keyboard-shortcuts-filter.spec.ts -- --retries=0
```

Before starting the mobile managed capture, copy this run's owned `.pr-assets`
screenshots and manifest to a unique external evidence directory, record their
hashes and inspect the desktop image. Do not overwrite or clean unowned assets.
Then run:

```bash
quiet e2e --summary -- env CAPTURE_PR_ASSETS=true pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/settings/mobile-keyboard-shortcuts-filter.spec.ts -- --retries=0
```

Preserve and hash its owned screenshots and manifest and inspect the phone image.
Record exact commands, results, screenshot paths/hashes, observed composition,
and native JOIN inventory. If capture ownership cannot be established, stop
before the runner clears anything. No live developer data, unowned processes,
volumes, caches, temporary files, or history fixtures may be cleaned.

## Files likely touched

- `apps/web/components/settings/keyboard-shortcuts-card.tsx`
- `apps/web/components/settings/keyboard-shortcuts-card.test.tsx`
- Optional focused helper and test under `apps/web/lib/keyboard/`
- `apps/web/e2e/tests/settings/shortcut-filter-helpers.ts`
- `apps/web/e2e/tests/settings/keyboard-shortcuts-filter.spec.ts`
- `apps/web/e2e/tests/settings/mobile-keyboard-shortcuts-filter.spec.ts`
- `docs/public/sessions-and-review.md`
- This plan, work order, and paired specification lifecycle statuses.

## Dependencies

None. A later separate implementation release and ROOT's HEAVY control are
required before execution; this work order does not authorize delegation.

## Risks

Hidden rows must not narrow conflict inputs or discard draft edits. Managed E2E
capture may clear owned assets; preserve evidence between runs. Recheck complete
fresh PR file inventory before publication and preserve concurrent edits.

## Parallelism

sequential

## Inputs

Paired requirement and design; `keyboard-shortcuts-card.tsx` and tests;
`sidebar-layout-editor-shortcut-picker.tsx`; settings-route E2E fixtures;
mobile UI language and control-sizing references.

## Results

- Frozen workspace install passed with pnpm 9.15.9. The first attempt failed
  before installation because PATH selected Node's Corepack pnpm 12.4.2;
  placing the pinned pnpm first resolved it.
- Mounted-card RED failed because the search textbox was absent; GREEN passed.
  Review remediation added two focus regressions: both failed before the fix;
  the final component suite passed 14 tests. Focused lint passed. Typecheck passed with an 8 GiB Node heap after
  the default 4 GiB heap exhausted. i18n check and ratchet, public-doc tests and
  validator, specification catalog and lint tests/linter, and diff check passed.
- Backend/delivery fixture, web E2E and plugin package builds passed at the
  tested source state. `chromium` passed two tests including the 767px
  fine-pointer case; `mobile-chrome` passed one. Each used one worker and zero
  retries. The phone capture was rerun after scrolling to show the result above
  the floating save bar; its lint and E2E test passed again.
- Desktop and phone images were inspected: the filter, clear action, matching
  row and conflict indicator fit the existing settings surface. Both original
  manifests and screenshots were retained externally with SHA-256 hashes before
  a subsequent managed capture could clear them. The phone row was verified
  above the save surface in the final capture.
- Fresh open-PR inventory covered 131 PRs and 188 file pages. Only three public
  document overlaps appeared; their inspected patches affected other sections.
  Repeat the inventory before publication if it becomes stale.

- Review remediation yields filter keystrokes from an active recorder and
  restores search focus after clear. Rebuilt web assets and reran the existing
  two Chromium and one mobile-chrome scenarios with real typing and clear focus;
  all passed. Fresh desktop and phone captures were inspected.
