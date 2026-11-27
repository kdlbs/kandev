---
id: "01-register-custom-font-drafts"
title: "Register and reconcile custom-font drafts"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-MANUAL-SAVE-001
acceptance_criteria:
  - AC-UI-SETTINGS-MANUAL-SAVE-001.1
  - AC-UI-SETTINGS-MANUAL-SAVE-001.2
  - AC-UI-SETTINGS-MANUAL-SAVE-001.4
system_design:
  - ../../specs/ui/system-design/settings-manual-save.md
---

# Task 01: Register and Reconcile Custom-Font Drafts

## Summary

Make each raw custom-font edit part of TerminalSettings' registered revision.
Preserve newer text/mode and sibling edits when an older successful save advances
the accepted baseline; prove the actual Save remains available and a second
explicit Save persists the newer font. Deliver code and independent regressions
together after the later implementation release.

## In scope

- `TerminalSettings`/`TerminalFontCard` parent draft ownership and immutable
  submission capture; accepted-boundary trimming and complete editor Reset.
- Raw blank/whitespace, preset-stack text, Custom/preset/Default transitions,
  unchanged saves, retry, and other terminal fields surviving ACK.
- One causal real-provider RED, two pre-fix passing controls, affected GREEN
  matrix, desktop/phone second-save E2E, and task-defined scoped checks.
- Synchronize this work order and manifest with actual results.

## Out of scope

Font loading/rendering/library, layout/markup changes, global coordinator/API
redesign, backend, new requirements or flags, unrelated publication races,
foreign resources, delegates/tasks/sessions/model switches. ROOT's protected
candidate must never be read, copied, or deleted; independently author tests.

## Acceptance

1. The causal regression fails before the fix at actual coordinator dirty/Save
   availability after ACK, with retained raw NewerFont and store SubmittedFont;
   both controls pass on that same production source. After the fix, the same
   test passes and second explicit Save persists NewerFont and becomes clean.
2. The transition matrix preserves the owning design's raw/mode/accepted-family
   distinctions, immutable normalization, failure retry, and local Reset. No
   input or Reset sends PATCH; preset/Default selection remains explicit.
3. Fresh managed desktop and mobile E2E prove enabled Save before second-action
   blur and second-save reload persistence; scoped checks pass with actual exits.

## ASCII UI preview

UI-01, shared desktop/phone terminal card after older ACK; see
[full plan preview and mobile composition](plan.md#ascii-ui-preview).

```text
Before: [Custom...] [NewerFont] -> clean, [Saved (disabled)]
After:  [Custom...] [NewerFont] -> dirty, [Reset] [Save changes]
```

Before Save consequence is source-derived; retained raw input/clean coordinator
are ROOT probe observations. Fixed shared action stays in its existing safe-area
position; card order and single content scroller are unchanged. Phone uses the
existing dedicated Terminal & Editors page. Maps to .1/.2/.4 above.

## Verification

ROOT completed the full design review and issued the later implementation
release with exclusive heavy lease113 on 2026-10-11. Execute in the same primary
session/model; preserve upstream changes. Use Node 24 on PATH.

### Once-only dependency bootstrap after release

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run once for this fresh worktree before any pnpm checks; do not repeat a completed
install. Retain returned session handles, wait to real exits, and record them.

### Independent RED and two controls, before production edits

Write `components/settings/terminal-settings-save.test.tsx` from the contract.
Use real StateProvider (seed loaded custom font), ToastProvider,
SettingsSaveProvider, TerminalSettings, and real updateUserSettings/fetchJson.
Stub only external PATCH fetch; return a valid 200 JSON response and fail any
unexpected network call. A read-only hook observer may inspect the actual store
and `useSettingsSaveCoordinator`; do not mock providers/hooks/client or invoke a
hand-built save callback. Clean up observers, deferred requests, fetch stubs,
React mounts, and navigation guards after every test.

For the causal test: edit/blur SubmittedFont, assert no PATCH before explicit
Save, press rendered Save and assert exactly one admitted pending PATCH carrying
SubmittedFont; type NewerFont while it is pending, keep custom input focused
without blur, resolve ACK inside act, then assert store SubmittedFont, visible raw
NewerFont, actual `hasDirty=true`, contributor dirty and enabled actual Save.
The pre-fix failure must be dirty/availability, not missing scaffolding or a
fabricated overwrite. After the fix, click the actual action again, assert second
PATCH NewerFont and accepted store, then clean contributor/no further Save needed.

```bash
(cd apps/web && pnpm exec vitest run components/settings/terminal-settings-save.test.tsx -t 'keeps newer unblurred custom font dirty and saves it explicitly')
(cd apps/web && pnpm exec vitest run components/settings/terminal-settings-save.test.tsx -t 'saves an ordinary custom font without newer edits|keeps newer parent-managed font size dirty after font ACK')
```

Record causal exit nonzero with the expected actual false-vs-true assertion;
both controls must exit 0 independently of the fix. Ordinary save asserts payload,
store and clean state without a newer edit. Font-size control changes size while
font PATCH is pending, asserts dirty/enabled action and the next explicit Save.

### Minimal production change and affected GREEN

Implement the owning design locally. Include raw input and mode in complete
revision; derive dirty values without mode-only noise. Keep Custom mode for blank
or preset raw strings until acceptance. Capture current raw input for Save, trim
nonempty text then, retain accepted candidate for blank, canonicalize unchanged
ACK only, and Reset complete editor from latest accepted baseline. Preserve newer
siblings. Independently add the transition/normalization/retry matrix named in
[the plan](plan.md#tests); no test that merely mirrors a helper implementation.

```bash
(cd apps/web && pnpm exec vitest run components/settings/terminal-settings-save.test.tsx components/settings/terminal-settings.test.ts components/settings/settings-save-provider.test.tsx components/settings/settings-save-revision.test.ts)
```

### Focused rendered desktop and phone checks

Author the two E2E files/helper listed below. Seed through apiClient; intercept
the first settings PATCH and use explicit request-admitted/response-release
signals, not elapsed waits. Preserve the real app coordinator/client. Assert the
input remains focused at first ACK and actual Save is enabled before click/tap
can blur it. Use the real backend response/persistence for second Save. Read the
existing settings-manual-save scenario and causal-waits helper for API cleanup
and causal request waits. Release pending routes in finally even on assertion
failure and restore only this test's font/size values. Inspect phone rendering;
assert shared action is reachable and no document horizontal overflow.

```bash
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --project chromium tests/settings/terminal-custom-font-save.spec.ts -- --grep 'keeps newer custom font available for a second explicit Save' --workers=1 --retries=0)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome tests/settings/mobile-terminal-custom-font-save.spec.ts -- --grep 'keeps newer custom font available for a second explicit Save' --workers=1 --retries=0)
```

Desktop command builds the final source; phone reuses those exact unchanged
artifacts immediately, serially. Rebuild if source changes between them. Confirm
test discovery/counts, actual command exits and runner teardown; no all-worker
overrides, overlapping heavy joins, or foreign runtime cleanup.

### Normal scoped validation and documentation gates

```bash
(cd apps/web && pnpm exec eslint --max-warnings 0 components/settings/terminal-settings.tsx components/settings/terminal-settings-save.test.tsx e2e/tests/settings/terminal-custom-font-save.spec.ts e2e/tests/settings/mobile-terminal-custom-font-save.spec.ts e2e/tests/settings/terminal-custom-font-save-helpers.ts)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/preserve-custom-terminal-font
```

The shared E2E helper is included in scoped lint. The design-shell Node limitation
was resolved by the supplied Node 24 implementation runtime. Evaluate the pure
exported documentation coverage check against the actual staged paths.
Before the actual authorized commit, stage only owned changes, run
`(cd apps/web && pnpm run i18n:ratchet)` against that real staged final diff,
including any new helpers, and retain actual hook exits. No copy additions are
planned. Run additional i18n catalogs only if copy changes are introduced.
Do not replay broad passing suites. Delivery/hosted observation remains governed
by ROOT's lease and later merge grant, not this completed local work order.

## Files likely touched

- `apps/web/components/settings/terminal-settings.tsx`
- `apps/web/components/settings/terminal-settings-save.test.tsx` (new)
- `apps/web/e2e/tests/settings/terminal-custom-font-save.spec.ts` (new)
- `apps/web/e2e/tests/settings/mobile-terminal-custom-font-save.spec.ts` (new)
- `apps/web/e2e/tests/settings/terminal-custom-font-save-helpers.ts` (new)
- `docs/specs/ui/system-design/settings-manual-save.md`
- `docs/specs/ui/README.md`
- This work order and `plan.md`.

## Dependencies

None. ROOT full design review and LATER explicit implementation interrupt/lease
are release gates, not an implicit approval from time passing or callbacks.

## Risks

Raw-family feedback can unmount Custom input; same-family Reset can miss mode
reconciliation; clicking the second Save can conceal the missing dirty revision
through blur. Constrain and assert all three boundaries. No API/global fix is
authorized if implementation encounters unrelated concurrency behavior.

## Parallelism

`sequential`. You are not alone in the repository: preserve other tasks' edits
and resources, adapting to current upstream. No native delegation.

## Inputs

- [Active requirement](../../specs/ui/requirements/settings-manual-save.md), .1/.2/.4.
- [Owning system design](../../specs/ui/system-design/settings-manual-save.md).
- [ADR 0046](../../decisions/0046-settings-route-save-coordinator.md).
- `terminal-settings.tsx`, nearby utility/provider tests, `StateProvider`,
  `ToastProvider`, real API client, existing desktop/manual-save mobile settings
  fixtures. ROOT protected candidate is excluded from inputs.
- ROOT causal evidence requalified on main 7229dbe15fb5f2aec37c9cf0c0f64fdd434e45ad.

## Results

Implemented and locally validated on 2026-10-11 after ROOT full design approval
and later implementation lease113. Parent draft owns raw text, Custom mode and
accepted family; immutable Save capture canonicalizes submitted text and ACK
preserves newer complete revisions. No coordinator/API/layout change or new copy.
The unchanged requirement and AC .1/.2/.4 govern the repair.

- Once-only frozen-lock install: native 49947, exit 0.
- Independent causal RED: native 46056, exit 1 at actual coordinator `false`
  versus expected `true`, with retained focused NewerFont and stored SubmittedFont.
- Two pre-production controls: native 89598, exit 0, both passed.
- Final affected GREEN command above: native 95777, exit 0, 43 tests/4 files.
- Final independently authored matrix after test-only typing correction:
  `pnpm exec vitest run components/settings/terminal-settings-save.test.tsx`,
  native 16259, exit 0, 14/14. Covers blank/whitespace fallback, preset raw
  acceptance on blur/Enter, explicit Default/preset, trimming, raw revert,
  same-family Reset, immutable ACK/newer sibling, second explicit save and retry.
- Final scoped ESLint command above: native 67430, exit 0.
- Original typecheck 2281 joined exit 2 (unsupported test locator option);
  corrected test typing, then typecheck 81025 joined exit 0. Original affected
  95777 and type 2281 were both joined before subsequent heavy work.
- Desktop managed E2E: original native 56161, exit 0, 1 chromium test passed
  (6.3s), one worker, retries 0. Fresh final build.
- Phone managed E2E: original native 79478, exit 0, 1 mobile-chrome test passed
  (5.8s), one worker, retries 0, `--no-build`. Before/after hashes identical:
  backend `ff393570875f3d9df5d5325521902eef0dbc0bfc734e8bdaac1b44639a7e3fb9`,
  web index `93a9fcee43410476381bc910a4761ee90d676f618e7f132a30842f11ab6f7036`.
  Both exact scenarios assert focused newer raw/dirty/enabled actual Save before
  second-action blur, second persisted PATCH, reload persistence, no horizontal
  overflow, and owned settings/route cleanup. Captures inspected on both viewports.
- Managed commands used the above scoped arguments through
  `../../scripts/run-quiet e2e --summary --` with an owned quiet-log directory,
  `/tmp/kandev-child113-terminal-font-20261011.xOdBzT`, and supplied Node 24 PATH.
  Logs: `kandev-run.e2e.5Bb1UJI5.log` (desktop),
  `kandev-run.e2e.5jqbeHwF.log` (phone). Runner exits joined; no live local handle.
- Public documentation review: `docs/public/developer-tools.md` remains accurate;
  existing manual-save semantics restored, no labels/routes/defaults/API changes.

- Final `python3 scripts/list-docs.py validate`: exit 0, 374 decisions/1528 specs.
- Final `python3 scripts/lint-spec-files.py --all`: exit 0. One-order/AC/capture
  mapping assertions and `git diff --check`: exit 0.
- Pure `validateCoverage` against all actual staged paths: exit 0, covered,
  accepted owning design/unchanged requirement and exactly this work order.
- Final staged `(cd apps/web && pnpm run i18n:ratchet)`: original native 97502,
  exit 0; 0 added/1 modified production file clean, 643 guard entries intact.
  All new test/helper paths were staged and included in the actual diff.

Both pre-commit and commit-msg hooks are installed and active. The normal commit
receipt and hosted exact-head evidence are retained in the external task delivery
checkpoint after their actual exits; no hook bypass or additional validation
phase. ROOT's later expected-head merge grant remains separate.


### Hosted review documentation correction

Corrected the owning design's stale closing note after automatic review and
made the Node prerequisite portable. Runtime, tests, requirement and captures
are unchanged. Targeted specification catalog/lint passed; normal commit hooks
and exact-head hosted delivery receipts remain in the external task checkpoint.
