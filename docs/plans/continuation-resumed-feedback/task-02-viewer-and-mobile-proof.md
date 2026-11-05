---
id: "02-viewer-and-mobile-proof"
title: "Prove resumed feedback across viewers and viewports"
status: completed
wave: 2
depends_on:
  - "01-confirmed-activity"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.6
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.6
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.7
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 02: Prove resumed feedback across viewers and viewports

## Summary

Update the existing continuation browser flows to verify neutral feedback while
the resumed turn remains active. Prove persistence, cancellation, and phone
usability, then update the public recovery explanation.

## In scope

- Desktop `read-hold` and phone `pending-v3-hold` flows in existing E2E files.
- Live resumed-output evidence, persisted marker, reload, and second viewer.
- Existing waiting Cancel, resumed Cancel, hidden prompt, and native trace checks.
- Phone hitbox, wrapping, overflow, and focused desktop/phone screenshots.
- The interruption-continuation explanation in `sessions-and-review.md`.
- Final work-order results and synchronized plan status.

## Out of scope

- New mock-provider capabilities or a native compatibility probe.
- New browser fixture framework, unrelated recovery flows, and full-suite audits.
- Additional production behavior beyond Task 01.

## Acceptance

1. Actual resumed output precedes the neutral card while the session remains
   RUNNING. The same persisted message and neutral state survive reload and a
   second viewer. Trace evidence shows one original prompt and one continuation.
2. Warning/counter assertions target the active card. They do not require removal
   of diagnostic history. Cancel works before dispatch and during resumed work.
   Existing internal-prompt, refusal, and native-identity checks remain effective.
3. Phone Cancel has at least a 44px hitbox, context wraps, and document overflow
   remains zero. Screenshots match UI-01/UI-02. Public guidance describes the
   neutral state and unchanged budget without claiming provider execution guarantees.

## ASCII UI preview

Excerpts from [the combined preview](plan.md#ascii-ui-preview), mapped to
`003.1`, `003.4` through `003.7`.

UI-01: Desktop Task > Chat, actual output after accepted continuation.

```text
[i] Agent resumed                         [Cancel]
    Cursor / selected model
    Previous history and tool results are preserved.
    <new output, continued turn remains active>
```

UI-02: Phone Task > Chat, the same persisted state.

```text
+--------------------------------------+
| [i] Agent resumed                    |
| Cursor / selected model              |
| Previous history and tool results    |
| are preserved.                       |
| [              Cancel              ] |
+--------------------------------------+
| <new output, turn remains active>    |
```

UI-03: Before actual activity, the warning and waiting/reconnecting labels remain.
The phone uses the existing stacked Cancel action. These labels are illustrative.
Keep Chat as the scroll owner and retain the current safe-area layout.

## Verification

Reuse `createContinuationFixture`, `waitForContinuationMessage`, and
`assertNativeContinuationTrace`. The existing hold fixtures send assistant text,
then wait for cancellation. Capture pre-continuation message counts or identities
so persisted text from an earlier turn cannot satisfy the resumed-output assertion.
Assert current session state through the API alongside rendered feedback.

Assert one neutral card, no active failure heading/counter, and visible Cancel.
Inspect the marker through persisted messages. Reload and repeat DOM assertions.
Open a second page and verify the same card state. Cancel through the UI and
assert settlement plus notice removal. Retain waiting cancellation separately.
Always dispose the fixture and close the second page in `finally`.

Task 01 owns the behavioral RED for this presentation defect. Browser assertions
follow that correction because the new persisted marker is part of its production
wiring. Record this reason rather than claiming a missing selector proves RED.

Run each managed E2E command sequentially from the repository root. The runner
builds fresh production artifacts and isolates its runtime. Do not overlap suites
or pass worker overrides. No browser check runs against the user's instance.

```bash
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium --project=mobile-chrome tests/session/provider-interruption-continuation.spec.ts tests/session/mobile-provider-interruption-continuation.spec.ts --list)
(cd apps/web && pnpm e2e:run --project chromium tests/session/provider-interruption-continuation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-provider-interruption-continuation.spec.ts)
(cd apps/web && pnpm exec eslint e2e/helpers/provider-interruption-continuation.ts e2e/tests/session/provider-interruption-continuation.spec.ts e2e/tests/session/mobile-provider-interruption-continuation.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/continuation-resumed-feedback
```

Before delivery, run this local coverage preflight from the repository root.
The representative production paths force coverage validation during design.
During implementation, also include the actual changed production paths.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/platform/requirements/provider-interruption-continuation.md',
  'docs/specs/platform/system-design/provider-interruption-continuation.md',
  'docs/plans/continuation-resumed-feedback/plan.md',
  'docs/plans/continuation-resumed-feedback/task-01-confirmed-activity.md',
  'docs/plans/continuation-resumed-feedback/task-02-viewer-and-mobile-proof.md',
];
const result = validateCoverage({
  changedFiles: [...paths, 'apps/backend/internal/orchestrator/event_handlers_streaming.go', 'apps/web/components/task/chat/messages/transient-retry-notice.tsx'].map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify({ok: result.ok, status: result.status, workOrders: result.workOrders, errors: result.errors}, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

Compare focused screenshots with the preview. Record screenshot paths, discovered
test counts, final test results, and any environmental blocker. Do not substitute
historical package counts for current results.

## Files likely touched

- `apps/web/e2e/tests/session/provider-interruption-continuation.spec.ts`.
- `apps/web/e2e/tests/session/mobile-provider-interruption-continuation.spec.ts`.
- `apps/web/e2e/helpers/provider-interruption-continuation.ts`, if shared
  persisted-message or viewer assertions need extraction.
- `docs/public/sessions-and-review.md` (explanation).
- `docs/plans/continuation-resumed-feedback/plan.md` and both work-order Results.

## Dependencies

Task 01. Confirm its current implementation and recorded unit results first.

## Risks

- The existing hold output says “accepted” but is actual assistant content.
  Do not confuse that marker with provider acceptance alone.
- A browser can retain stale message state until message-updated hydration.
  Pair DOM assertions with persisted metadata and a second viewer.
- Global failure-text searches can match retained history. Scope presentation
  assertions to the single current recovery card.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/provider-interruption-continuation.md).
- [System design](../../specs/platform/system-design/provider-interruption-continuation.md).
- [Task 01](task-01-confirmed-activity.md).
- Existing continuation E2E helpers, SessionPage controls, native trace assertions,
  and managed runner guidance in `apps/web/e2e/README.md`.

## Results

Updated the desktop `read-hold` and phone `pending-v3-hold` flows and the public
recovery guidance.

- Discovery:
  `(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium --project=mobile-chrome tests/session/provider-interruption-continuation.spec.ts tests/session/mobile-provider-interruption-continuation.spec.ts --list)`
  found 20 tests: 16 in `chromium` and 4 in `mobile-chrome`.
- `(cd apps/web && pnpm e2e:run --project chromium tests/session/provider-interruption-continuation.spec.ts)`
  passed 16 tests in 2.5 minutes. The desktop flow asserted actual resumed
  output, the neutral card, visible Cancel, the absent counter, the persisted
  marker, reload, a second viewer with the same message ID, and settlement after
  Cancel.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-provider-interruption-continuation.spec.ts)`
  passed 4 tests in 39 seconds. The phone flow asserted the neutral card, visible
  touch-sized Cancel, the persisted marker, reload, no horizontal overflow, and
  settlement after Cancel.
- Focused reruns after the final assertion edit passed 1 desktop and 1
  phone test.
- Screenshots were captured and attached as `desktop continuation running` and
  `phone continuation running` under each backend temporary directory; the E2E
  runner removes those directories during disposal.
- `(cd apps/web && pnpm exec eslint e2e/helpers/provider-interruption-continuation.ts e2e/tests/session/provider-interruption-continuation.spec.ts e2e/tests/session/mobile-provider-interruption-continuation.spec.ts)`
  reported no problems.
- `node --test scripts/validate-public-docs.test.mjs` passed 63 tests;
  `node scripts/validate-public-docs.mjs` validated 54 published pages.
- `python3 scripts/list-docs.py validate` validated 424 decisions and 1861
  specifications. `python3 scripts/lint-spec-files.test.py` passed 36 tests and
  `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed. The local coverage preflight returned `covered` for
  both work orders with zero errors.
