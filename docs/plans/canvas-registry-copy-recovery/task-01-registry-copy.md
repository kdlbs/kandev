---
id: "01-registry-copy"
title: "Recover registry example copy"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CANVASES-MARKETPLACE-003
  - REQ-CANVASES-MARKETPLACE-006
acceptance_criteria:
  - AC-CANVASES-MARKETPLACE-003.5
  - AC-CANVASES-MARKETPLACE-006.1
system_design:
  - ../../specs/canvases/system-design/marketplace-sharing.md
---

# Task 01: Recover registry example copy

## Summary

Use the existing dialog-safe clipboard utility in Canvas sharing and reflect its
actual result. Prove the rejection recovery in a component regression before
fixing production, then verify the real Share flow on desktop and phone.

## In scope

- Independently author real-component tests with real locales. Assert exact
  displayed example passed to copy, review-derived ID and fallback ID as
  applicable, modern success, rejected modern write with legacy success,
  missing modern API with legacy success, and failure of both paths. After a
  success followed by failure, ensure no stale success label; capture unhandled
  rejection evidence. Restore all mocked browser boundaries after each test.
- Run the new test before editing production; record the actual exit and
  defect-specific assertion failure. A setup error or missing selector is not RED.
- Use `copyToClipboard` locally and set copied state to its boolean. Keep all
  example content, metadata, i18n, navigation, and layout unchanged.
- Extend desktop and mobile sharing specs with actual actions and clipboard data
  for normal success and controlled modern rejection. Mobile uses taps. Use
  isolated fixture feature enable/restore, seed/promote/remove and cleanup on
  failure. Only mock the modern Clipboard API for rejection; do not fake DOM
  copy or clipboard success. Read permission on the owned browser context is
  allowed. Assert actual clipboard readback matches the displayed example after
  each successful action. Preserve the real DOM fallback. If the native clipboard
  is unavailable, collect and report evidence before changing test scope.
- Both current sharing `test.describe` blocks have no file-level retry override;
  add `test.describe.configure({ retries: 0 })` inside each affected describe
  during implementation, and check any nested or later configuration does not
  re-enable retries. CLI `--retries=0` alone cannot override a describe setting.

## Out of scope

New UI copy, toast, timeout, registry writes, external provider actions, flag
profile changes, global helper changes, protocol, layout, and unrelated cleanup.

## Acceptance

- Actual component regression establishes causal RED and passes after the fix,
  including success, absent API, rejection recovery, and total failure controls.
- Both desktop and phone actual Share flows copy the displayed example with the
  modern path and with controlled modern rejection when native clipboard works.
  Report any native clipboard limitation without claiming untested success.
- No false copied feedback or unhandled rejection on total failure. Preserve
  existing display and Share behavior.

## Verification

Commands run from repository root, serially only after the separate heavy grant.
Each invocation creates a fresh task-scoped quiet directory, including every
rerun; preserve every native handle and actual exit. Keep the original
workspace and dependencies.
For a fresh worktree install once; do not install before the grant. New test name
is intentionally outside the reviewed browser project list so it runs in
`browser-locales` with the real locale. No jest-dom matcher assumptions.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-child035-install.XXXXXXXX)" scripts/run-quiet install -- pnpm --dir apps install --frozen-lockfile
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-child035-red.XXXXXXXX)" scripts/run-quiet vitest -- pnpm --dir apps/web exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism components/settings/canvas-share-help-clipboard.test.tsx
# Make the production correction only after confirming causal RED.
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-child035-green.XXXXXXXX)" scripts/run-quiet vitest -- pnpm --dir apps/web exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism components/settings/canvas-share-help-clipboard.test.tsx
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-child035-desktop.XXXXXXXX)" scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/canvas/canvas-sharing.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-child035-phone.XXXXXXXX)" scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/canvas/mobile-canvas-sharing.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-child035-eslint.XXXXXXXX)" scripts/run-quiet lint -- pnpm --dir apps/web exec eslint --max-warnings 0 components/settings/canvas-share-help.tsx components/settings/canvas-share-help-clipboard.test.tsx e2e/tests/canvas/canvas-sharing.spec.ts e2e/tests/canvas/mobile-canvas-sharing.spec.ts e2e/tests/canvas/canvas-clipboard-helpers.ts
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-child035-catalog.XXXXXXXX)" scripts/run-quiet catalog -- python3 scripts/list-docs.py validate
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-child035-spec-lint.XXXXXXXX)" scripts/run-quiet spec-lint -- python3 scripts/lint-spec-files.py --all
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-child035-diff.XXXXXXXX)" scripts/run-quiet diff -- git diff --check
KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-child035-status.XXXXXXXX)" scripts/run-quiet status -- git status --short
```

If a shared E2E helper is added, include its exact path in the ESLint command.
The phone command builds afresh; `--no-build` is permissible only after proving
unchanged source, tool and backend configuration fingerprints. Run canonical
web typecheck only if the implementation requires it or new diagnostics demand
it. Re-run only checks affected by later edits. Inspect rendered desktop/phone
outcomes and record capture if required for the PR. Assess `docs/public/canvases.md`,
root README and `docs/screenshots.md` against the final change.

## Files likely touched

- `apps/web/components/settings/canvas-share-help.tsx`
- `apps/web/components/settings/canvas-share-help-clipboard.test.tsx`
- `apps/web/e2e/tests/canvas/canvas-sharing.spec.ts`
- `apps/web/e2e/tests/canvas/mobile-canvas-sharing.spec.ts`
- An E2E helper only if necessary to share native clipboard setup.

## Dependencies

None. Implementation requires a later explicit request and heavy grant.

## Risks

Native clipboard availability varies; a controlled API rejection is not a real
permission-policy denial. A false-positive DOM copy stub would invalidate proof.

## Parallelism

Sequential.

## Inputs

Marketplace requirement and design, existing share component, clipboard utility,
Canvas sharing specs and owned fixture. Do not use private discovery fixtures.

## Results

- Frozen workspace install: exit 0. Independently authored component RED: two
  rejection-path assertions failed and two controls passed; Vitest recorded two
  corresponding unhandled rejections. A prior run had a separate test selector
  mistake, corrected before this RED. After the fix, all four cases passed.
- Desktop chromium and phone mobile-chrome managed runs: one passed each with
  one worker and zero retries. Both used real clipboard readback for normal copy
  and controlled modern API rejection; the fallback used real DOM copy. The
  first phone run timed out because the test tried to reopen an already-open
  actions sheet; after correcting the interaction the phone run passed.
- Scoped ESLint passed with zero warnings. The initial lint run found a repeated
  test literal; after extracting it, the component suite and lint passed again.
- Catalog validation, specification lint, and diff checks passed. Public
  `docs/public/canvases.md` already describes the existing sharing workflow;
  no public documentation or screenshot change is needed for this repair.
- Desktop and phone captures were taken from the isolated managed fixture after
  controlled rejection and verified against clipboard readback. This does not
  establish a real browser permission-policy denial.
