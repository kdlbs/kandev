---
created: 2026-09-18
status: done
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
  - REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001
system_design:
  - ../../specs/tasks/system-design/clarification-active-lifecycle.md
  - ../../specs/tasks/system-design/clarification-response-reliability.md
legacy_specs: []
---

# Implementation Plan: Dismiss inactive clarification questions

## Overview

Repair the confirmed inactive-response path associated with
[issue #3798](https://github.com/kdlbs/kandev/issues/3798).
One work order updates shared response handling and proves desktop/phone recovery.
Implementation is complete.

## Evidence and root cause

Investigation base: `26254fe51f`. The report names `57bbae10ad` but provides no
network trace or confirmed reproduction. Both revisions wire the X-shaped
`ClarificationSkipButton` to rejection, separately from local collapse.

`useClarificationGroup` classifies `409 not_active` as `expired`. It updates
message status only for `ok`. `useResolveCallback` also handles only `ok`.
The overlay renders its expired banner beside enabled question actions.
Without an authoritative message update, the question remains pending locally.
Further X clicks repeat the request instead of removing the obsolete panel.

A temporary test copied the existing overlay harness and clicked X twice.
Both responses returned `409 {"code":"not_active"}`. After both settled, the
test confirmed two requests, `rejected=true`, and no message-store update.
Its final removal assertion failed because X remained mounted and enabled.
The temporary test was removed; permanent regressions are recorded in Task 01.

This proves one repairable path, not the cause of every reported occurrence.
Timeouts, failed requests, and malformed responses remain distinct outcomes.

## Requirement conformance

- `AC-TASKS-CLARIFICATION-LIFECYCLE-001.2` requires obsolete questions to stop
  being answerable. Reuse this active requirement without creating a repair spec.
- `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.1` distinguishes expiry from
  acceptance and retryable failure.
- `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.4` preserves retry controls
  after actual failures. Do not make every unsuccessful response disappear.
- The response-reliability design now specifies cache reconciliation. Existing
  authority and persistence decisions remain unchanged.

## Scope

### In scope

- Retire the submitted inactive bundle from the latest client message cache.
- Preserve terminal siblings, newer bundles, and authoritative restoration.
- Remove response controls from a static host's expired overlay.
- Cover task chat on desktop and phone, plus shared hook and overlay behavior.

### Out of scope

- Backend mutations, new endpoints, database migrations, and agent cancellation.
- Changing Escape/collapse semantics or treating expiry as successful rejection.
- Redesigning the Inbox, task summaries, or the clarification layout.
- Claiming that this repairs an unobserved timeout or backend failure.

## Technical approach

In `apps/web/hooks/domains/session/use-clarification-group.ts`, extend the
existing response reconciliation for `expired`. Read the current store rows
before changing only pending rows from the submitted bundle to local `expired`.
Match message ID, session ID, and pending ID. Preserve current metadata and
terminal siblings; never insert a missing row from the submitted snapshot.
Use `isPendingClarificationMessage` for the existing legacy pending semantics.
Compare `updated_at` before applying expiry so a newer authoritative restoration
wins. If the request generation is stale while the same pending ID is current
again, skip the cache write; an old bundle remains eligible for retirement while
a different pending ID is active.

Keep response identity separate from current UI identity. An old request may
retire its own stale rows but cannot change a replacement bundle's controls,
outcome callback, or in-flight guard. Preserve the existing generation fence.
Do not store expiry in the backend or suppress later authoritative restoration.

In `clarification-input-overlay.tsx`, render only the existing expired notice
when static props retain an inactive bundle. Do not register active response
shortcuts in that state. Keep `no_longer_active` distinct from `resolved` and
leave `onResolved` success-only. Guard direct submission/retry calls after
expiry until bundle replacement or an authoritative restoration resets state.

Task chat, Quick Chat, and run transcripts derive pending questions from the
shared message cache. Their existing selectors remove the obsolete panel.
The Inbox retains its existing `no_longer_active` handling. No host-specific
successful-response callback substitutes for cache reconciliation.

## ASCII UI preview

UI-01: Task chat after X receives `409 not_active`, desktop and phone.

```text
Before                       After
[Question              X v]  [Conversation / question history]
[Question no longer active]  [Message composer              ]
[Answer choices           ]
[Message composer         ]
```

The stale panel disappears; history remains. A static host can show the expired
notice without response controls. The diagram uses illustrative copy.
Phone retains the existing focused chat and inline composer, with no new sheet.
`ClarificationPanelSection` and `mobile-clarification.spec.ts` are the nearest
shipped exemplars. Keep current scroll ownership, safe-area handling, and
44px coarse-pointer targets for remaining controls. Shared state owns expiry.
Map this view to lifecycle AC `.2` and response-reliability AC `.1`.

## Tests

- `use-clarification-group.test.ts`: inactive Skip updates only matching
  pending rows; preserve terminal siblings and unrelated bundles in one fixture.
- `use-clarification-group.regressions.test.ts`: delayed A response after B
  mounts, deleted rows, latest metadata preservation, authoritative restoration,
  and direct resubmission guards.
- `clarification-input-overlay.test.tsx`: `X retires an inactive bundle without
  another rejection request`; replace the existing expired-banner expectation
  with a non-actionable notice and no success callback.
- Existing timeout, retry, malformed-response, success, and collapse tests
  remain required. These cover response-reliability AC `.1` and `.4`.

## E2E tests

Add a scenario tagged `inactive dismissal` in `e2e/tests/chat/clarification.spec.ts`
and `e2e/tests/chat/mobile-clarification.spec.ts`. Use `chromium` and
`mobile-chrome`, respectively. Intercept the exact response request with
`409 not_active`, while retaining the stale pending message in the client.
Arm the HTTP causal wait before clicking/tapping X. Assert panel removal,
composer availability, and no second rejection request. A newer bundle must
remain answerable. Also retain existing real-backend Skip success coverage.

## Work orders

- [x] [Task 01: Reconcile inactive clarification responses](task-01-reconcile-inactive-responses.md)

Execute sequentially. No delegation is required or authorized.

## Verification results

Temporary reproduction: failed at the expected control-removal assertion.
The existing hook, overlay, and panel suites passed: 3 files, 73 tests.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-clarification-group.test.ts components/task/chat/clarification-input-overlay.test.tsx components/task/chat/clarification-panel-section.test.tsx)
```

Documentation catalog validation passed: 290 decisions and 1015 specifications.
The full specification lint initially exposed a pre-existing duplicate
`AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.8` in
`docs/specs/tasks/requirements/queued-session-ownership.md`; the unrelated
conversation-surface criterion was renumbered to `.003.10` so the catalog has
unique acceptance IDs. The full lint then passed. `git diff --check` passed.

The normal commit hooks passed without a bypass. The issue assignment was
verified as `carlosflorencio`.

## Implementation results

Task 01 is complete. The hook now reconciles only current pending rows from the
submitted inactive bundle, preserves newer metadata and terminal siblings, and
blocks repeated actions until replacement or authoritative restoration. A newer
authoritative `updated_at` wins over a delayed inactive response, and stale
A→B→A generations cannot expire the bundle that is current again. Static expired
overlays retain only the non-actionable notice and do not arm their Escape
guard. Desktop and phone E2E cover the same intercepted inactive response.

The browser-first regression failed before the correction at the expected
store-update and stale-control assertions. After the correction, validation
passed:

The review regressions also failed before the correction: the delayed restored
bundle became `expired`, the A→B→A path wrote an expired row, and the expired
overlay still claimed Escape. Each now passes.

- Focused Vitest: 5 files, 94 tests.
- Targeted ESLint: no errors or warnings.
- Prettier, TypeScript typecheck, `make build-web`, `make build-backend`, and
  `git diff --check` passed.
- Desktop inactive-dismissal E2E: 1 test passed.
- Phone inactive-dismissal E2E: 1 test passed.
- E2E fixture plugin packaging passed.
- Documentation catalog validation passed: 290 decisions and 1015
  specifications.
- Full specification lint passed after the pre-existing duplicate acceptance
  ID was corrected to `AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.10`.

## Related delivery records

[Response reliability](../clarification-response-reliability/plan.md) and
[active lifecycle](../clarification-active-lifecycle/plan.md) remain historical
delivery records. This follow-up does not change their completed scopes or results.

## Risks

- A 409 is not proof that this caller rejected the question. Preserve outcome identity.
- Another caller's failed delivery can restore pending state. Authoritative
  restoration must remain possible; do not add permanent local suppression.
- A delayed response must not overwrite newer metadata or terminal siblings.
- The report lacks runtime evidence. Investigate separately if the failure
  persists with a successful response after this focused repair.

## Documentation impact

This package records implementation intent. No public UI labels, API contract,
or operator procedure changes; public documentation does not need an update.
