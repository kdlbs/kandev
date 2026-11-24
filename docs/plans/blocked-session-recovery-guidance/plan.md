---
created: 2026-11-11
status: in_progress
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-TASKS-SESSION-TAB-RESTART-001
system_design:
  - ../../specs/agents/system-design/session-recovery-guidance.md
  - ../../specs/tasks/system-design/session-tab-restart.md
legacy_specs: []
---

# Fix plan: Blocked session recovery guidance

## Overview

[Issue 5259](https://github.com/kdlbs/kandev/issues/5259) reports recovery controls that hide or discard the server's history-continuation action.
Deliver typed block-resolution feedback, preserve the action in uncertain delivery, then connect Restart to the same guidance owner.
These are implementation violations of existing criteria, not a new recovery policy.

Issue 5259 is assigned to `carlosflorencio`.
The canonical report and both comments on [issue 5256](https://github.com/kdlbs/kandev/issues/5256) were read on 2026-11-11.
Neither report contains image attachments.
The inspected checkout is `bceeaf1c1d3`, two commits after the report's `a268e5c4090`.

## Confirmed root cause and reproduction

1. `orchestrator.Service.resolveSessionRecoveryBlock` returns a plain error when native recovery cannot prove resolution.
   The handler requires `RecoveryReason()` and recognizes only four reasons.
   The original block classification is lost, and `unknown_prompt_outcome` is outside that mapping.
2. `useStoppedRecoveryChoices` returns early for uncertain delivery.
   Its restricted builder never includes `actions.continuationDetails`, even when the server supplied them.
3. `useSessionRestart` catches the structured `session.launch` error and retains only its message in a toast.
   The hook-local continuation state in another recovery consumer never receives those details.

A temporary component regression used the existing `BannerHarness` and injected a generation-2 `session_restore_required` hint.
Its reason was `unresolved_durable_work` and its action was `continue_from_history`.
The continuation button existed in the ordinary banner.
After rerender with `uncertainDelivery`, Retry connection remained and the continuation button disappeared.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/issue-5259.repro.test.tsx)
```

Result: one expected failure, `retains continuation guidance when delivery is uncertain`.
The final assertion could not find `recovery-continue-from-history-button`.
The temporary file was removed after the run.
Task 02 makes this regression permanent before implementation.

Read-only tracing confirmed the backend and Restart paths.
No affected host, live database, or agent journal was modified.
The reported OOM and repeated-restart sequence was not reproduced against a real backend during planning.

## Requirement conformance

The active [Agents requirement](../../specs/agents/requirements/session-recovery-failures.md) owns valid actions and recovery presentation.
Applicable criteria are 006.2, .4-.6, .9, .12, .14-.18, and .25.
The active [Tasks requirement](../../specs/tasks/requirements/session-tab-restart.md) owns the Restart sequence.
Its criterion 001.3 explicitly requires normal recovery actions after failed Resume.
Criteria 001.2, .4-.6 preserve navigation, pending state, phone access, and drafts.
No acceptance criterion is rewritten or duplicated.

Agents owns the guidance design because it owns recovery eligibility across all entry points.
Tasks retains Restart lifecycle ownership.
Platform retains durable proof, generation allocation, and interrupted-submission disposition.

## Scope

### In scope

- Typed feedback for an unchanged current block whose native resolution fails.
- The existing continuation action for canonical `unknown_prompt_outcome` and existing supported reasons.
- Consistent action visibility in stopped, uncertain-delivery, and composer recovery surfaces.
- Store-scoped, incident-bound continuation guidance shared by Restart and ordinary recovery.
- Desktop and phone behavior, localization, regression tests, and public recovery guidance during implementation.

### Out of scope

- Issue 5256's native proof, stale recovery metadata, duplicate-block, and continuation-bootstrap repairs.
- A new discard/reset operation, automatic continuation, manual database editing, or journal deletion.
- Panel polling, terminal idle-reclaim loops, and changes to Stop's backend effect.
- New recovery pages, menus, feature flags, or permission policies.

## Technical approach

### Typed feedback

Follow [typed server guidance](../../specs/agents/system-design/session-recovery-guidance.md#typed-server-guidance).
Retain the canonical current block's typed reason after failed resolution.
Re-read before returning actionable details.
A replaced block, failed read, unsupported cause, or generic provider error cannot authorize the previous action.
Preserve concurrent authenticated resolution and every unresolved submission's historical uncertainty.

Keep the existing WebSocket error kind and action.
Extend the mapper's reason allowlist with `unknown_prompt_outcome`.
Canonical refusals include optional block and incarnation identities to support projection-before-response ordering.
The continuation endpoint remains responsible for admission and checkpoint safety.
The frontend validates optional target and generation fields before accepting the hint.

### Shared presentation

Task 02 repairs the uncertain-delivery early return without adding unrelated recovery operations.
Use one continuation-choice helper for the affected adapters.
Show continuation first when the matching server hint requests it.
Reuse standard action sizing, pending feedback, and existing profile and guard checks.

Task 03 moves continuation guidance into the store-scoped request model described by the design.
It stores validated details and identity only.
Existing durable failure history keeps its current owner.
Restart publishes the captured launch failure, and the active recovery owner consumes it after the menu unmounts.
A failed continuation retains its matching hint until recovery succeeds or its incident changes.

| Input or provider condition | Behavior and verification |
| --- | --- |
| Compatible server returns the typed continuation hint | Offer one explicit action. Handler, service, component, and browser regressions cover routing. |
| Canonical current delivery block is `unknown_prompt_outcome` | Native proof refusal returns the existing explicit continuation hint. Endpoint admission remains authoritative. |
| Older response omits session or generation | Accept only within the captured current request. Do not infer another target. |
| Generic provider, authentication, or transport error | Preserve safe failure feedback. Do not invent continuation eligibility from its wording. |
| Foreign session, superseding block, or generation mismatch | Ignore stale guidance. Preserve the current incident and pending guards. |
| Supported native restore succeeds | Retire matching controls. Do not replace the native conversation or replay the old prompt. |

This is provider-neutral UI routing over explicit backend evidence.
It does not expand a provider's native-resume capability.

## ASCII UI preview

UI-01: Stopped or uncertain session after a matching continuation hint.
The before view is supported by source and the temporary component reproduction.

```text
BEFORE, uncertain delivery
+-----------------------------------------------------+
| Message delivery is uncertain                       |
| Resume failed                                       |
| [Resume, if eligible] [Retry connection] [Stop]       |
+-----------------------------------------------------+

AFTER, desktop
+-----------------------------------------------------+
| Message delivery is uncertain                       |
| Saved history starts a new native conversation.      |
| The interrupted prompt outcome remains unknown.      |
| [Continue from saved history] [other eligible actions]|
| [Stop]                         [Technical details v] |
+-----------------------------------------------------+

AFTER, phone
+----------------------------------+
| Message delivery is uncertain    |
| Saved history starts a new       |
| native conversation. The old     |
| prompt outcome remains unknown.  |
| [Continue from saved history   ] |
| [Other eligible action         ] |
| [Stop                          ] |
| [Technical details v]            |
+----------------------------------+
```

UI-02: Restart fails with a typed continuation hint.

```text
Desktop: session-tab menu > Restart session
Phone: Sessions picker > visible row actions > Restart session

BEFORE: menu closes > error toast only
AFTER:  menu closes > request feedback + UI-01 in captured session
```

UI-03: Pending, failed, and unavailable actions, shared composition.

```text
Pending:     [Continue from saved history (disabled)]
             Continuing... (accessible status)
Failed:      Continuation failed. (localized failure)
             [Continue from saved history] (explicit retry)
Unavailable: Prerequisite explanation, no enabled forbidden action
```

The drawings specify hierarchy and action order, not exact copy or pixel spacing.
All new copy is localized.
UI-01 maps to Agents 006.2, .6, .12, .14-.18.
UI-02 maps to Tasks 001.2-.5 and Agents 006.5, .9, .16-.17.
UI-03 maps to Agents 006.4, .9, .16, .18.

The existing composer recovery region is the mobile surface.
`SessionRecoveryCard` and `RecoveryActions` supply its hierarchy and controls.
The phone Sessions picker supplies Restart's visible entry point.
This short recovery flow stays inline rather than opening another overlay.
The bounded recovery region owns expanded-content scrolling, with no nested diagnostic scroller.
Existing dynamic viewport and safe-area handling remain in place.
Phone and coarse-pointer actions measure at least 44 pixels.
Desktop actions retain 28-pixel sizing and wrap.
Tests cover narrow fine-pointer widths and no horizontal document overflow.

## Tests

| Criteria | Planned regression evidence |
| --- | --- |
| Agents 006.2, .12, .18, .25 | `TestRecoveryBlockResolutionGuidance` and `TestRestoreRequiredRecoveryResponseRecoveryReasons` distinguish current canonical blocks from stale and untyped failures. |
| Agents 006.4-.6, .12, .14-.18 | `session-stopped-banner.test.tsx`: `retains continuation guidance when delivery is uncertain`, plus dispatch, guard, missing-profile, and pending assertions. |
| Agents 006.5, .9, .16 | `session-recovery-guidance.test.ts`: projection-before-response ordering, incident replacement, generation mismatch, late reply, failure retention, and owner isolation. |
| Tasks 001.2-.6 | Restart hook and provider tests: structured guidance survives menu unmount without changing target, selection ownership, draft, or pending semantics. |

## E2E tests

Extend `e2e/helpers/history-continuation-recovery.ts` and its existing desktop/phone wrappers.
Scenarios cover stopped recovery, uncertain delivery, and failed Restart.
Assert no continuation request before explicit activation and exactly one request afterward.
Exercise disabled and failed attempts, wrapped copy, phone hitboxes, focus, and navigation races.
Extend existing `session-tab-restart` coverage for menu-specific entry points.
Transport-intercepted success proves request routing only.

The release check uses a disposable real backend after issue 5256's repair is integrated.
It seeds the repeated-interruption state through the existing delivery fixture boundary.
It verifies continuation bootstrap, current generation binding, saved history, and admission of one distinct new prompt.
The interrupted submission must remain non-replayable.
All unresolved sibling records must retain their independent gates.
This check is required before claiming that the affected sessions have a working recovery path.

## Work orders

- [x] [Task 01: Preserve typed block-resolution guidance](task-01-typed-block-guidance.md)
- [x] [Task 02: Retain continuation in uncertain delivery](task-02-uncertain-delivery-actions.md), after 01
- [ ] [Task 03: Share Restart recovery guidance](task-03-restart-guidance-handoff.md), implementation complete; real-backend acceptance pending issue 5256

Execute sequentially in the primary conversation.
The package did not itself authorize implementation or delegation. The user later explicitly requested implementation in this task; execution remains sequential.

## Verification results

Planning evidence, 2026-11-11:

- The temporary reproduction failed once for the expected missing continuation button. Its file was removed.
- The existing five focused Vitest files passed: 62 tests.
- The two existing durable-recovery handler tests passed.
- The issue assignment was read back from GitHub and confirmed as `carlosflorencio`.

Baseline commands:

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/session-stopped-banner.test.tsx components/task/chat/session-recovery-card-history.test.tsx hooks/domains/session/use-session-restart.test.ts lib/services/session-recovery-service.test.ts hooks/domains/session/use-session-recovery-revision.test.ts)
(cd apps/backend && go test -trimpath ./internal/orchestrator/handlers -run 'TestRestoreRequiredRecoveryResponseExposesUnresolvedDelivery|TestWSLaunchSession_DurableRecovery' -count=1)
```

Implementation verification, 2026-11-11:

- Focused frontend recovery Vitest run passed: 14 files, 97 tests. Typecheck and lint passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed.
- `make -C apps/backend build` passed. The targeted orchestrator handler and core recovery tests passed.
- Task 02 desktop and phone continuation E2Es passed, 2/2 each.
- Task 03 phone Restart E2E passed, 3/3. The new desktop Restart handoff passed; the six-test desktop suite had one existing real-backend interrupted-turn failure because the card was missing after `unknown_prompt_outcome`. This path is blocked by open issue 5256.
- Public-doc validation passed: 63 tests and 55 documents. Spec validation passed: 448 decisions and 1982 specifications; all specification files passed lint.
- `git diff --check` passed after recording the implementation results.
- The real-backend fixture scenario remains unrun until issue 5256 is integrated. No successful backend continuation is claimed.

Review fixes, 2026-11-11:

- Resume resolution now performs the canonical supported-block classification after generic fallback refusals as well as native-proof refusals. Missing-proof tests cover both supported reasons and verify persisted outcomes and admission are unchanged.
- Recovery guidance is retired and fenced by same-generation recovery evidence, observed block removal, or an authoritatively RUNNING session. For non-running sessions, missing projections remain pending so the typed failure handoff still works.
- Provider and hook regressions cover external native recovery, stale hints with a healthy composer, projection-before-success, and late failure fencing. The focused suite passed 15 files and 100 tests; typecheck, lint, i18n gates, web/backend builds, and scoped Go lint passed.
- Task 03 remains incomplete for real-backend acceptance until issue 5256 is integrated. Its acceptance scenario has not been run, and no real recovery pass is claimed.

Package validation passed:

- `python3 scripts/list-docs.py validate`: 448 decisions and 1982 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed, including byte limits.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: covered all three work orders with four accepted requirement/design references.
  This was prospective implementation coverage using the banner path and the package's actual documents.
  No product file was changed to run this check.
- Relative links and whitespace passed in all five new documents.
- `git diff --check` and package status checks passed.

At the end of planning, the ten design documents were unstaged and uncommitted.
The original Restart package keeps its completed status and historical results.
Task 03 updated the public session recovery guidance during implementation.

## Risks and dependency

- Issue 5256 is open. Its generation-binding failure can prevent the newly visible action from completing.
- The backend repair is outside this package. Task 03's real-backend release check depends on its integration.
- A visible action and an intercepted successful reply cannot prove real recovery.
- Incorrect hint lifetime can transfer an old action to a later incident. Identity regressions are mandatory.
- Canonical recovery and session error files are close to their size limits. Keep this guidance in its capability-specific design.
- The minor polling and Stop observations are not claimed as repaired.

## Public documentation

`docs/public/sessions-and-review.md` now explains the explicit action after failed Restart and uncertain delivery.
It describes history continuation's new native conversation and preserved uncertainty without promising that real-backend recovery always succeeds.

## Related delivery records

- [Original Restart package](../session-tab-restart/plan.md) retains its completed results. This package extends its failure handoff.
- [Recovery UI package](../session-error-recovery-ui/plan.md) retains active composer ownership.
- [Recovery convergence package](../session-recovery-convergence/plan.md) retains durable reconciliation authority.
