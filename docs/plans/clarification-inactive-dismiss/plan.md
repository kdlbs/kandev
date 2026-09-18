---
created: 2026-09-18
status: complete
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
  - REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001
system_design:
  - ../../specs/tasks/system-design/clarification-active-lifecycle.md
  - ../../specs/tasks/system-design/clarification-response-reliability.md
legacy_specs: []
---

# Implementation Plan: Close or answer inactive clarification questions

## Overview

Repair the confirmed inactive-response path associated with
[issue #3798](https://github.com/kdlbs/kandev/issues/3798) and preserve a useful
conversation path after the original clarification waiter ends. Task 01
reconciles inactive responses, race ordering, and Escape behavior. Task 02
supersedes its removal-only presentation with late answers delivered as
ordinary messages through the source task and session.

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

## Revised contract and delivery scope

The user's latest direction takes precedence over the historical Task 01
approach below. Reuse `REQ-TASKS-CLARIFICATION-LIFECYCLE-001`, amended criteria
`.1`/`.2` and new criteria `.4` through `.9`. Requirements, design, and the
[late-answer ADR](../../decisions/2026-09-18-late-clarification-messages.md)
now separate operational tool responses from ordinary messages.

- Preserve earlier questions in transcript history with an answer-as-new-message action.
- Preserve submitted answers across `409 not_active` and send through normal admission.
- Keep X/Close as dismissal without a new message for inactive questions.
- Reuse existing sending/steering/queue rules; no new busy-agent policy is introduced.
- Keep Inbox History read-only and use its existing source-conversation navigation.
- Keep Task 01's race and Escape fixes wherever applicable.
- Task 02 owns UI, admission adapter, localization, and focused regression coverage.

UI-02: Earlier question, shared desktop and phone inline composition.

```text
[Earlier question                                  ]
[Answer as new message                             ]
       opens:
[Question and answer choices                       ]
[Custom answer                                     ]
[Close]                       [Send as new message  ]
[Failure: draft retained, Retry available           ]
```

A recognized inactive result during answer submission uses the same delivery
path with the captured draft. Sent or queued admission replaces the form with
its actual outcome. Closing returns focus to its opener. Keep the existing
phone scroll owner and safe areas; controls have 44px coarse-pointer targets.
This view covers lifecycle criteria `.4` through `.9`. Exact spacing is illustrative.

The technical approach and results below retain Task 01's historical repair
details and record Task 02's revised implementation separately.

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

- Reconcile submitted inactive bundles without overwriting newer authority,
  terminal siblings, or restored pending rows.
- Keep expired static overlays free of response controls and preserve the
  existing Escape, collapse, and generation fences.
- Offer historical unanswered questions an answer-as-new-message flow on
  desktop and phone.
- Preserve affirmative answers after recognized inactive responses and deliver
  them through ordinary message admission with stable retry identity.
- Localize the new actions and feedback, update public task guidance, and
  validate the shared hook plus desktop/mobile browser flows.

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
Compare `updated_at` independently for each row before applying expiry so a newer
authoritative restoration wins while unchanged pending siblings remain eligible.
If the request generation is stale while the same pending ID is current again,
skip the cache write; an old bundle remains eligible for retirement while a
different pending ID is active.

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

For Task 02, the shared late-answer adapter captures the source task/session,
question bundle, and answers before invoking `useMessageHandler`. It reuses
ordinary input-mode and queue admission, keeps the captured client message ID
across remounts, and exposes sent, queued, and retryable failure outcomes.
Historical transcript rows open the same inline question form in explicit
new-message mode. The source conversation remains the routing authority.

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
  with a non-actionable notice and no success callback. Cover answer retention
  and late-message success, queue, and retry feedback.
- `clarification-request-message.test.tsx` and the formatter tests cover the
  historical action, current-turn exclusion, question context, selected
  labels, custom text, and system-marker neutralization.
- `use-message-handler.test.ts` covers sent/queued outcomes, a current
  clarification barrier, unavailable transport, and caller-owned retry IDs.
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

Add `late answer` scenarios to the same desktop and phone files. Assert that a
historical question can open the form, that the admitted message contains the
question and answer, and that coarse-pointer controls remain reachable. Keep
the active submission fallback covered by the submit-failure scenario.

## Work orders

- [x] [Task 01: Reconcile inactive clarification responses](task-01-reconcile-inactive-responses.md)
- [x] [Task 02: Send late answers as new messages](task-02-late-answer-messages.md)

Execute sequentially. Task 02 depends on Task 01 and revises its user-facing outcome.

## Verification results

The temporary reproduction failed at the expected control-removal assertion.
The existing hook, overlay, and panel suites passed: 3 files, 73 tests.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-clarification-group.test.ts components/task/chat/clarification-input-overlay.test.tsx components/task/chat/clarification-panel-section.test.tsx)
```

Documentation catalog validation passed: 291 decisions and 1015 specifications.
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

- Task 02 focused Vitest: 8 files, 142 tests passed.
- Targeted ESLint: no errors or warnings on changed frontend and E2E files.
- Targeted Prettier, TypeScript typecheck, `make build-web`, `make build-backend`, and
  `git diff --check` passed.
- Desktop historical late-answer E2E: 1 test passed.
- Phone historical late-answer E2E: 1 test passed.
- Active 409 late-answer fallback E2E: 1 test passed.
- E2E fixture plugin packaging passed.
- Documentation catalog validation passed: 291 decisions and 1015
  specifications.
- `i18n:check` passed for all supported catalogs. The all-locale
  `i18n:zh-hant` helper still stops on the two pre-existing simplified
  `workflows.openAgentSettings` entries; the changed `task` namespace converted
  successfully for both Traditional Chinese catalogs.
- Full specification lint passed after the pre-existing duplicate acceptance
  ID was corrected to `AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.10`.
- PR documentation coverage passed after nesting the late-answer criteria under
  the lifecycle requirement heading.
- The review follow-up now compares message versions with `parseTurnTimestamp`,
  preserving nanosecond ordering and rejecting malformed timestamps. Expiry is
  selective per row, so an unchanged pending sibling still retires beside a
  newer restored row. Companion parking work orders reference the corrected
  `.003.10` criterion.

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

Task 01 did not need public documentation changes. Task 02 must document the
late-answer action and its normal send/queue behavior in the existing task
conversation guide. This is recorded in `docs/public/tasks-and-workflows.md`;
queued admission is described as queueing, not agent receipt.
