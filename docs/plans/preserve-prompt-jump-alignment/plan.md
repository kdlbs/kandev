---
created: 2026-10-10
status: in_progress
requirements:
  - REQ-UI-PINNED-PROMPT-AVAILABILITY-001
system_design:
  - ../../specs/ui/system-design/pinned-prompt-availability.md
legacy_specs: []
---

# Implementation Plan: Preserve prompt jump alignment

## Overview and checkpoint

Retain the reader's explicit start target through delayed automatic older-page
commits. UI owns this reusable transcript navigation contract. Reuse the unchanged
[owning requirement](../../specs/ui/requirements/pinned-prompt-availability.md),
especially AC-UI-PINNED-PROMPT-AVAILABILITY-001.2; reconcile its existing design
and implement one bounded order. No new requirement or assertion relaxation.

ROOT full actual-file review and LATER implementation INTERRUPT released this
package under exclusive GLOBAL heavy grant111. Scroll implementation and affected
checks are complete; batched hooks/publication await the separately reviewed
Agents CI correction. SAME task
`450a75ba-e760-464e-b185-2b3eb1f407b4`, session
`da87b7fb-c6ac-4d31-8a41-df529c2e5237`, SAME primary; no delegates/new sessions.
Preserve committed Global fix/PR4398 head `a589325537d2b70ff66f02f289c1d1ad62f88127`.

## Evidence and reconciliation

Required E2E run38037433650/attempt1, job114171494755 (E2E Shard 6/14), fails
`retains an unloaded last prompt after reload and navigates from both desktop controls`.
ONE independent managed diagnostic, native17938 actual exit1/5628d0, reproduced
the first desktop landing without retries. Before activation: target unloaded,
overlay open/92px. Afterwards: correct row visible, rowTop97.375, viewportTop80,
own margin92px, closed overlay, delta -74.625px. The around GET succeeds, followed
by automatic older GETs; the latest prepend follows the 250ms reassertion. The
native prepend path uses the explicit id only while the transient motion guard
is locked, then preserves the early clamped visual anchor. Visibility alone does
not satisfy the existing start-position criterion. Second desktop control is
not reached by the failing case and still needs GREEN evidence.

Implicated test/helper/chat sources are byte-identical across candidate, exact
tested merge0443cfa0d60fdb1037a698ce038a97aa5e661eff, and exact observed main19237eb.
Temporary diagnostic instrumentation was checksum-restored, all known owned
processes absent, tree clean, heavy RETURN accepted. Evidence lives under
`/tmp/kandev-global-secrets-design-20261010/`; no diagnostic source becomes a test.

The existing [availability package](../pinned-prompt-availability/plan.md) remains
its historical implementation record; this order supplies the missing delayed
prepend coverage for its navigation contract. The independent
[Global package](../preserve-saved-global-secrets/plan.md) remains locally complete.
Required Frontend failures are separately classified in the external checkpoint;
no agent-catalogue product behavior is part of this scroll package.

Original hosted native42336 actually joined exit1/4a50c0 after3295s/49polls:
42 passed, 6 failed, 0 pending, complete errors[]. Wrapper/group absent; preserve
original clock/receipts. Shard8 job114171494712 timed out at configured45m with
no final test names or artifact; cause UNKNOWN, no blind retry or timeout inflation.
No old-head rerun; a later necessary fix publication triggers fresh CI.

## Scope

### In scope

- One local active start target and existing reader claim across automatic prepend.
- Existing reader intent, cancel, supersession/latest, session/placement and list isolation.
- Exact desktop first/second controls and phone unloaded prompt alignment.
- Ordinary prepend, center navigation, clamped range, reduced motion and follow controls.

### Out of scope

General history/lifecycle frameworks, new store/API/public handle, projection or
around-window machines, timers/guard extension, pagination/cursors/backend,
overlay/layout/copy/new interactions, dependencies, broad tests/builds, retries,
or rewriting the Global production change. No protected/foreign resource edits.

## Technical approach

Only production path: `apps/web/components/task/chat/message-list-native-scroll.ts`.
Lift its existing local `startAlignedTargetRef` to the native management composition
so existing real reader-intent/explicit reader-claim and cancellation routes can
clear it. Keep the programmatic claim callback separate. Pass it through private
prepend/message-scroll helpers; retain it after motion-lock release and use it for
automatic prepend correction while current and rendered. Clear at the existing
session/placement boundary before layout work. Return the composed reader claim
through the unchanged handle shape so existing divider/imperative readers yield
the target without editing `message-list-native.tsx`. Preserve fallback anchoring
when absent, no-target behavior, and browser clamping. No new export or dependency.

## ASCII UI preview

UI-01: Desktop transcript, unloaded last prompt jump via bar or status control.

```text
Observed after delayed prepend:       Required landing after delayed prepend:
+-- transcript viewport top --+       +-- transcript viewport top --+
| [selected prompt] +17.375px  |       | row's own margin: 92px      |
| older/newer rows scroll     |       | [selected prompt] +92px     |
+----------------------------+       | older/newer rows scroll     |
| existing status controls   |       +----------------------------+
                                     | existing status controls   |
```

UI-02: Phone transcript, existing status control is the entry point.

```text
+-- full-height transcript ---+
| row's own computed margin   |
| [selected prompt]           |
| older/newer rows scroll     |
+----------------------------+
| existing compact controls   |
+-- safe-area / composer -----+
```

AC-UI-PINNED-PROMPT-AVAILABILITY-001.2/.8 require actual row-own-margin alignment
and phone absence of the desktop bar; spacing above is illustrative. Preserve the
single transcript scroll owner, localized copy, existing controls, dynamic
viewport/safe area and touch targets. Reuse the shipped phone chat/status surface
and mobile UI language's full-height chat composition. Loading uses the existing
jump indication; empty, error, and competing-owner behavior stay as specified.
This changes scroll interaction: the Global data-only mobile exception does not
apply. No new operator steps or gestures.

## Tests

`apps/web/components/task/chat/message-list-native.test.tsx` gains a focused
`retained start target` group. Independently simulate real native management:
select start target with own nonzero margin, settle/release motion and verifier,
commit automatic prepend after250ms, then assert exact relative bounds/scrollTop.
Parameterize successive page commits and reduced motion. Against unchanged source
this must causally fail with ordinary no-target prepend controls passing.

| Existing AC | Evidence / compatibility |
| --- | --- |
| AC-UI-PINNED-PROMPT-AVAILABILITY-001.2 | delayed-prepend RED/GREEN; wheel/key/touch intent yields; explicit cancel, newer start/center/absent target, latest, session change, two-list isolation; existing canceled-scroll/clamped-range verifiers |
| AC-UI-PINNED-PROMPT-AVAILABILITY-001.7 | ordinary visual anchoring/height delta and successive-page controls; no new requests/cursor edits |
| AC-UI-PINNED-PROMPT-AVAILABILITY-001.8 | unchanged phone alignment E2E and touch-intent control |
| AC-UI-PINNED-PROMPT-AVAILABILITY-001.1/.3/.4/.5/.6/.9 | unchanged resolution/gates/projection; exact desktop/phone E2E prove applicable unloaded controls and existing overlay composition |

## E2E tests

- Chromium: `apps/web/e2e/tests/chat/last-prompt-scroll.spec.ts`, exact failed
  `retains an unloaded last prompt after reload and navigates from both desktop controls`;
  both fresh-reload activations must pass existing own-margin assertion and retain
  newest rows (.1/.2/.3/.7/.8).
- mobile-chrome (Pixel5): `apps/web/e2e/tests/chat/mobile-last-prompt-scroll.spec.ts`,
  `phone reaches an unloaded last prompt without rendering the desktop bar`;
  existing precise alignment assertion remains unchanged (.1/.2/.7/.8).

## Work orders

- [ ] [Task 01: Retain the explicit start target](task-01-retain-start-target.md)

## Verification results

DESIGN_READY, 2026-10-10. Catalogue validation passed (369 decisions /1513
specifications); all specification lint passed. Actual-path/link/reference
preflight `check-ci-design.cjs` passed: requirement unchanged, all nine ACs
defined, one requirement/design/order, existing production path explicitly
covered with zero errors. Actual three changed Git paths are unstaged docs,
with docs-only coverage `exempt` recorded separately from that production-path
`covered` result. Raw unfiltered NUL inventory has111176 entries, exactly two
new manifest/order paths, no removals/unexpected additions/foreign Git edits.
`git diff --check` passed; both added-file no-index whitespace checks emitted
no diagnostics (their exit1 reports added content). Receipts are under the
evidence directory. This is the historical design checkpoint; implementation
results below do not replace its original receipts.

### Released implementation results

- Independent causal RED: native43719 joined1/de6a24, two motion variants
  failed104px versus required92px after motion-lock release; eight controls passed.
  Earlier runs retained:27263 fake-timer setup failure;74491 causal geometry plus
  timer restoration/session-fixture failures. Corrected fixtures reran before
  production; no failed setup was accepted as qualification.
- One production module. Existing placement latches own one local start id and
  reset it at their session/token boundary; prepend uses it beyond motion-lock
  release. Composed reader claims and existing cancel/supersession/latest paths
  clear it. Private callback/parameter reuse keeps lint limits without suppression,
  new store or public API.
- Final affected selection:25PASS/71skipped, native51984 actual0/bb92c6.
  Preceding25PASS runs17511/65717 followed scoped fixture/readability changes.
  Earlier31549 exposed the fixture omitting the actual divider reader-intent
  callback with auto-scroll disabled;98846 exposed an incorrect latest-position
  control expectation. Fixture precision corrected; causal92px assertion retained.
- Unchanged desktop E2E native47502 joined0/fc33bb:1PASS20.9s, both bar/status
  activations. Unchanged Pixel5 phone E2E native54317 joined0/66e60c:1PASS19.5s,
  exact own-margin alignment and no desktop bar. Serial one-worker/retries0,
  ordinary managed fresh prerequisite builds; no install or unrelated suite.
- Scoped lint37247 joined0/d2ab78 after resolving line limits/duplicate literal/
  unused destructure warnings. i18n check18775 joined0/49ba04; orphan warnings
  informational. Pinned-base ratchet56205 joined0/b5f6f2, no new copy.

Full command/process/native evidence and E2E logs remain under the evidence
directory. Desktop run-quiet used its default `/tmp` log directory by mistake;
full log was preserved and the phone command used the owned directory. No blind
rerun or cleanup concealed it. Normal hooks/commit/batched publication remain
pending Agents review/release and correction; PR is not READY.

## Risks

- A retained id must yield before reader input or new placement; stale correction
  would undo a deliberate scroll or affect another session/list.
- Clearing on every programmatic claim/scroll would lose the accepted jump;
  lifetime must follow existing ownership, not the transient animation guard.
- Shard8's unknown timeout and the separately classified Frontend leaf remain
  delivery blockers; this package does not claim to resolve unrelated causes.


### Batched delivery checkpoint

Scroll local checks and both exact existing desktop/phone E2E actual JOIN0 were
completed before the separately reviewed Agents correction. The authorized exact
upstream prerequisite merged normally without conflicting with either scroll file.
No passing scroll/browser checks were replayed solely for main drift. Required
Agents affected cases and scoped checks are now complete; batch publication and
fresh hosted acceptance remain. Original failed observer actual JOIN1 and clock
are retained; no old-head rerun.
