---
id: "01-retain-start-target"
title: "Retain the explicit start target"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PINNED-PROMPT-AVAILABILITY-001
acceptance_criteria:
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.1
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.2
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.3
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.4
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.5
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.6
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.7
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.8
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.9
system_design:
  - ../../specs/ui/system-design/pinned-prompt-availability.md
---

# Task 01: Retain the explicit start target

## Summary and checkpoint

Keep an accepted start-aligned row selected through automatic older-page commits
after native motion settles. Yield on existing reader intent/cancel/supersession
and session boundary. ROOT full actual-file review and LATER implementation
INTERRUPT released this scope under exclusive GLOBAL heavy grant111. Separate
Agents remediation remains DESIGN ONLY; publication waits for its resolution.
SAME task/session/primary as [manifest](plan.md), no delegation or file parallelism.

## Inputs

- [Unchanged requirement](../../specs/ui/requirements/pinned-prompt-availability.md):
  .2 outcome; .7 window/cursor boundary; .8 phone parity; others compatibility.
- [Owning design](../../specs/ui/system-design/pinned-prompt-availability.md#retaining-start-alignment-through-automatic-prepend).
- Native management/reader intent, prepend capture, `useScrollToMessage`, existing
  guard/verifier/clamp tests, unchanged exact desktop and phone cases.
- Actual diagnostic native17938 exit1/restored checksum/RETURN, original hosted
  native42336 exit1/4a50c0/current owned absence; full evidence in manifest.

## In scope

One instance-local explicit target, private helper wiring, existing reader claim,
delayed prepend correction and narrowly affected independent tests. Keep current
Global fix/public head until later authorized batched CI-remediation publication.

## Out of scope

New history/lifecycle owner/store/public API, pagination or around-load changes,
layout/copy/new interaction, timeout/guard inflation, assertion weakening, broad
verification, installs/cache wipes, foreign/protected resources, unrelated Agents
product behavior. No production comment narrating ACs or CI history.

## Acceptance

1. Independent delayed-prepend test fails causally against unchanged production
   after guard/verifier settle, ordinary prepend controls pass, and minimal
   correction makes affected tests GREEN. Real intent/cancel/new target/latest/
   session/list isolation prevent stale correction. Preserve center/clamp/follow
   and reduced-motion behavior through existing affected controls.
2. Only production file below changes, preserving public handle, current target
   machines, requests/cursors, control composition and existing errors. Both
   desktop controls and phone pass their existing exact row-own-margin assertions.
3. Retain full original native results, every yielded handle and actual joins;
   actual unfiltered paths/coverage and normal scoped checks/hooks pass before
   one authorized push. No manual FULL BOT merely for SHA change or blind retry.

## ASCII UI preview

Excerpt of [full preview](plan.md#ascii-ui-preview), AC-UI-PINNED-PROMPT-AVAILABILITY-001.2/.8:

```text
UI-01 Desktop                     UI-02 Phone
+-- transcript top --------+      +-- transcript top --------+
| row's own margin (92px)   |      | row's own computed margin|
| [selected prompt]        |      | [selected prompt]        |
| remaining rows scroll    |      | remaining rows scroll    |
+-------------------------+      +-------------------------+
| existing status controls |      | compact controls/composer|
```

Desktop bar/status jump remains; phone has no bar. Existing single scroll owner,
safe area, localized controls and jump-loading indication remain. No data-only
mobile exception; scrolling is an interaction. Reader intent takes precedence.

## Files and ownership

- Production: `apps/web/components/task/chat/message-list-native-scroll.ts` only.
- Permanent affected tests: `apps/web/components/task/chat/message-list-native.test.tsx`.
- Unchanged rendered acceptance: `apps/web/e2e/tests/chat/last-prompt-scroll.spec.ts`,
  `apps/web/e2e/tests/chat/mobile-last-prompt-scroll.spec.ts` and existing helper.
- Artifacts: owning design, this order and manifest; requirement reused unchanged.

You are not alone in the codebase. Preserve others' edits/worktrees/deps/caches/
refs/processes/protected resources. Capture tracked/cached/untracked NUL paths
unfiltered; expected paths only compare. Report unexpected changes before expanding
scope. Docs-only exemption is not production coverage. No new TS helper planned;
stage any later reviewed new TS helper before final existing i18n ratchet.

## Implementation sequence

1. After later release, mark in_progress; use existing Node24/managed deps (already
   installed once). Add focused `retained start target` group using native management
   harness. Independent RED, ordinary controls; no copied diagnostic instrumentation.
2. Minimal local ref/private wiring. Retain accepted target across motion release;
   clear through existing actual reader intent, explicit claim/cancel/new request/
   latest/session-placement boundary. Preserve ordinary absent-target fallback.
3. Run affected GREEN, scoped lint/i18n, then TWO exact rendered cases serially,
   one worker/retries0 with ordinary managed runner and owned log directory. Its
   necessary fresh Vite/backend/fixture prerequisite builds only under the granted
   lease; no separate broad build. No browser/system install without reconciled blocker.
4. Record actual results/update artifacts and other required CI corrections under
   their own order; normal hooks/commit/push, current owned cleanup and heavy RETURN.
   Hosted continuation only after actual local/publication joins/RETURN, using ROOT's
   original clock and retry budgets. No old-head rerun. READY/merge remain distinct;
   merge needs separate ROOT static compatibility check and expected-head serial grant.

## Verification

From repo root under runtime Node24, sequentially, only after later release:

```bash
(cd apps/web && corepack pnpm@9.15.9 exec vitest run components/task/chat/message-list-native.test.tsx -t 'retained start target|anchors a prepend|anchors a stored prompt|anchors each committed prepend|freezes the prepend baseline|canceled-scroll landing|superseded verifiers|absent superseder|supersedes an in-flight prompt verifier|cancels pending navigation landing')
(cd apps/web && corepack pnpm@9.15.9 exec eslint components/task/chat/message-list-native-scroll.ts components/task/chat/message-list-native.test.tsx --max-warnings 0)
(cd apps/web && corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && corepack pnpm@9.15.9 run i18n:ratchet)
KANDEV_RUN_QUIET_DIR=/tmp/kandev-global-secrets-design-20261010 scripts/run-quiet e2e --summary -- env -u KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM -u KANDEV_ALLOW_UNSAFE_TEST_PARALLELISM CI=true corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project chromium -- e2e/tests/chat/last-prompt-scroll.spec.ts --grep 'retains an unloaded last prompt after reload and navigates from both desktop controls$' --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR=/tmp/kandev-global-secrets-design-20261010 scripts/run-quiet e2e --summary -- env -u KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM -u KANDEV_ALLOW_UNSAFE_TEST_PARALLELISM CI=true corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project mobile-chrome -- e2e/tests/chat/mobile-last-prompt-scroll.spec.ts --grep 'phone reaches an unloaded last prompt without rendering the desktop bar$' --workers=1 --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run focused new group for RED before production; the complete listed selection
for GREEN covers every new/changed native case and existing affected controls.
Reference/path/PR-docs preflight uses actual changed paths plus explicit production
coverage, all ACs/design refs, ONE order; repeat after implementation scope changes.
Design allows only catalogue/spec/links/whitespace/inventory preflight now.

## Dependencies

None; ROOT design review and later explicit release/heavy admission are execution gates.

## Risks

Clearing the target too soon loses geometry; clearing too late undoes reader input.
Current session/layout reset must precede correction, and sibling lists must not share it.

## Parallelism

`sequential`

## Results

Scroll implementation/affected checks complete; see
[manifest results](plan.md#released-implementation-results): qualified causal
RED,25affected unit controls GREEN, scopedlint/i18n/ratchet PASS, unchanged
desktop and phone E2E each PASS/oneworker/retries0. Original native joins and
current-owned cleanup/RETURN recorded externally. Normal hooks and batched
publication await separately released Agents correction; order stays in_progress.


### Batched delivery checkpoint

Scroll local checks and both exact existing desktop/phone E2E actual JOIN0 were
completed before the separately reviewed Agents correction. The authorized exact
upstream prerequisite merged normally without conflicting with either scroll file.
No passing scroll/browser checks were replayed solely for main drift. Required
Agents affected cases and scoped checks are now complete; batch publication and
fresh hosted acceptance remain. Original failed observer actual JOIN1 and clock
are retained; no old-head rerun.
