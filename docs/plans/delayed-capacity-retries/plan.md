---
created: 2026-10-21
status: complete
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-003
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
legacy_specs: []
---

# Implementation plan: Delayed capacity retries

## Overview

Extend eligible retained-runtime capacity recovery beyond the existing five short
attempts. The user chose a maximum thirty-minute recovery window, waits capped
at five minutes, and manual continuation after backend restart. Platform owns
the scheduling and feedback contract across runtime, orchestration, and Chat.

This is a timing amendment to the existing
[requirement](../../specs/platform/requirements/transient-turn-runtime-continuity.md)
and [design](../../specs/platform/system-design/transient-turn-runtime-continuity.md).
The specification frontmatter retains its existing accepted lifecycle; the
October 21 amendment is implemented and locally verified through this package.

## Evidence

The screenshot reports a Codex ACP capacity failure with generic stopped-retry
copy. It cannot establish whether admission was refused or attempts ran out.
No live task was modified or diagnosed from historical messages.
Before this amendment, the shared retry policy used five attempts and waits
totaling 135 seconds, including capacity exhaustion after five dispatches.
`provider_interruption_restart.go` retires process-local ownership on restart.
The existing capacity evidence and same-runtime continuation path can support a
longer wait without broadening its safety policy.

## Scope

In scope: eleven capacity retry slots, fixed thirty-minute admission window,
single-owner cancellation and deadline handling, selected-policy notice metadata,
truthful stop feedback, deterministic unit tests, desktop/phone behavioral proof,
and public recovery guidance.

Out of scope: model/provider switching, persistent retry jobs, new settings or
release flags, new provider capabilities, general transient-policy changes,
runtime restoration after completed effects, and repairing the pictured task.

## Technical approach

Extend the existing `transientRetryEntry`; retain `handleTransientFailure` as the
single scheduler. Transfer immutable capacity-policy start/deadline and counters
across successor entries. Retain the global five-attempt fallback for other
policies. Implement the exact eleven-delay sequence in the owning design.
Use one policy-aware budget predicate at scheduling, timer fire, admission, and
terminal disposition. Revalidate ownership, authorization, runtime identity,
configuration, generation, queued input, and effect safety immediately before send.
Neither output nor a replay-to-continuation transition resets the budget.

Persist `retry_window_ends_at` only as presentation metadata; restart does not
reconstruct retry authority. Extend existing startup notice cleanup to accept
eleven started capacity attempts. Reuse `TransientRetryNotice`, `ActionMeta`,
and `interruption-recovery-feedback.ts` for deadlines and stop reasons. Localize
new text across all seven real locales, generating Traditional Chinese and pseudo.

Inject clock/timer dependencies locally for service tests. Browser fake time
alone cannot test backend scheduling. Any E2E accelerator must be wired solely
through isolated test construction and unavailable to production. Reuse the
existing real ACP mock and trace assertions for execution/native identity and
initialization counts. Avoid a 30-minute real-time E2E test or a public clock API.

### Compatibility

| Provider/shape | Transport and evidence | Intended behavior | Proof/fallback |
| --- | --- | --- | --- |
| Codex capacity continuation | Existing ACP typed retained runtime and capacity snapshot | Extended episode on same native conversation | Service/ACP mock traces; absent or unsafe evidence remains manual |
| Eligible pre-result capacity replay | Existing ACP replay fence plus retained runtime binding | Same extended budget, no output/effects | Service negative and replay-to-continuation tests |
| Other providers with pre-result capacity | Only if already eligible with complete retained-runtime evidence | Same policy, no new capability inferred | Conditional service fixture; unsupported shapes remain manual |
| Older remote agentctl | Missing typed retention/capacity evidence | No extended recovery admission | Omission regression tests |
| Cursor transport/resource exhaustion, network, quota/auth | Existing classifier and owner | Existing policy and five-attempt limits | Existing short-retry and continuation tests |
| Office, automation, dynamic, utility, passthrough | Independent owners | Existing policy | Existing exclusion fixtures |

## ASCII UI preview

UI-01: Current task Chat while capacity recovery waits, desktop:

```text
Model at capacity. Retrying in 2:00.             [Cancel]
Codex / selected model
Attempt 6 of 11. Recovery window ends at 14:30.
```

UI-01: Same state, phone:

```text
Model at capacity.
Retrying in 2:00.
Codex / selected model
Attempt 6 of 11.
Recovery window ends at 14:30.
[ Cancel ]
```

UI-02: Window expired, desktop and phone (wrapping naturally):

```text
Model at capacity.
Automatic recovery window ended.
11 attempts started. You can send another message.
[ normal composer ]
```

Copy/counts/times are illustrative and localized; a window may end with fewer
attempts. Required structure: one current inline notice, condition before timing,
correct policy maximum, separate window deadline, reachable Cancel, and truthful
terminal reason. Reuse the shipped `transient-retry-notice.tsx` within
`session-mobile-layout.tsx`; no new overlay. Phone wraps text and uses a 44px
Cancel target; desktop retains compact controls. Chat owns transcript scrolling.
The composer retains safe-area clearance and draft/focus behavior. Announce
phase changes without announcing every countdown tick. Maps to AC `.5` and `.7`.

## Tests

Task 01 owns these exact new tests in `provider_interruption_capacity_delayed_test.go`:

- `TestCapacityDelayedRetryScheduleAndLimit`: AC `.3`, `.6`, eleven delays/cap,
  five-attempt non-capacity regression, and unchanged reset-hint scope.
- `TestCapacityDelayedRetryDeadline`: AC `.3`, deadline equality, slow dispatch
  preparation, late timer, no-fitting-slot wake, and active-turn completion.
- `TestCapacityDelayedRetryOwnership`: AC `.1`, `.2`, `.4`, `.8`, cancellation,
  queued input, changed settings/auth, unsafe tools, runtime loss, duplicate/stale
  callbacks, changed error class, and replay/continuation transitions.
- `TestCapacityDelayedRetryRestartRetirement`: AC `.7`, surviving runtime,
  eleven-attempt metadata, one retired notice, and zero restart dispatches.

Extend `interruption-recovery-feedback.test.ts`, `transient-retry.test.ts`, and
`action-message.test.tsx` for deadline parsing, old metadata fallback, selected
maximum, actual started count, and separate refusal/expiry/exhaustion copy (AC `.5`).
Preserve all existing capacity and retained-runtime regression suites.

## E2E tests

Task 02 creates `e2e/tests/session/delayed-capacity-retries.spec.ts` (chromium)
and `mobile-delayed-capacity-retries.spec.ts` (mobile-chrome). Shared helpers
prove AC `.1` through `.8` through these scenarios:

- Capacity persists beyond five attempts, then succeeds on the same process,
  execution, and native conversation with preserved completed tools.
- Cancel during the extended wait and send a new user message; no old retry
  fires. Actual phone Cancel hitbox is at least 44px.
- Reload and a second viewer show one current notice without extra dispatches.
- Expire the admission window with fewer than eleven dispatches; show truthful
  stopped feedback and usable composer. Separate budget exhaustion case.
- Restart during an extended wait; no restored countdown or automatic prompt,
  including with a surviving retained runtime. Preserve history and manual input.
- Mixed completed/uncertain work remains manual with zero automatic attempts.

Use causal backend/WS observations, no arbitrary sleeps. Phone checks include
wrapping, viewport containment, no horizontal document overflow, and draft
preservation. Capture focused desktop and phone screenshots for UI-01/UI-02.

## Work orders

- [x] [Task 01: Extended capacity policy and feedback](task-01-capacity-policy-and-feedback.md)
- [x] [Task 02: Behavioral proof and recovery guidance](task-02-recovery-proof-and-guidance.md)

Execute sequentially, 01 then 02. No delegation is authorized.

## Verification results

Local implementation checks passed on October 21:

- Backend: `go test -trimpath -race ./internal/orchestrator ./internal/backendapp -run 'Test(Capacity|Transient|AgentTurnFailed|Retained|Continuation|InterruptionContinuation)' -count=1`.
- `scripts/lint-go-changed` on the changed orchestrator and backend composition
  packages: zero issues.
- Frontend: 65 focused tests across interruption feedback, transient retry, and
  action-message rendering passed; affected source and E2E lint passed with
  warnings treated as errors. Frontend typecheck passed with a 6 GiB Node heap.
- `pnpm run i18n:check` passed across all locales.
- Managed Chromium: 8 tests passed across delayed-capacity and transient-runtime
  continuity specs. Managed mobile-chrome: 8 tests passed across corresponding
  phone specs, including eleven-attempt exhaustion, touch cancellation, reload,
  restart, same-runtime recovery, and mixed-work refusal.
- Browser runs used `--host --no-build` after rebuilding `build`,
  `build-e2e-delivery-fixture`, and `e2e-plugin-package`; unchanged Vite assets
  came from the preceding managed production build. Artifact freshness gates
  remained enabled. Suites ran sequentially with one worker.
- Fresh waiting and expired-window screenshots were captured on desktop and
  phone, inspected, compressed, and validated against `.pr-assets/manifest.json`.
- Specification catalog and lint, public documentation validation and tests,
  PR documentation coverage, and whitespace checks passed.

Browser Red exposed two missing continuation invariants: a clean continuation
failure must not require another completed tool, and every continuation must
retain the original initiating user in its prompt evidence. Unit regressions
reproduced both failures before their corrections. Fresh complete safety evidence,
accepted execution/generation, conversation identity, and final authorization
checks remain mandatory; the original completed request is never replayed.

No live task was changed or subagents launched. Remote CI/review/merge evidence
is handled by the authorized PR delivery workflow after the local commit.

## Risks

- The generic screenshot may represent a safety refusal; delayed retries must
  never bypass that refusal merely because the displayed error says capacity.
- Increasing the shared constant would silently broaden other recovery owners.
- Restart cleanup currently assumes five attempts in historical notice parsing.
- A timer can fire on time but dispatch late; the final admission check is required.
- Browser time mocking cannot substitute for backend clock control.
- Keeping a runtime idle longer retains its existing execution-slot allocation.

## Public documentation impact

Task 02 updated the recovery explanation in `docs/public/tasks-and-workflows.md`
with limits, Cancel, manual fallback, and restart behavior. Earlier completed
packages remain historical evidence for their original behavior.

## PR remediation verification

Review regressions now prove that a fifth transport-restoration attempt can
finish its already-counted admission, and rejected retained-runtime replays do
not increment the actual started count. Budget checks remain at new-attempt
admission; owner validation still enforces deadline, identity, and authorization.
Restart retirement clears pending timing/actions and attempts stale-action cleanup
if conversion fails. Changed-error settlement is covered through the terminal
handler inside and after the original capacity window. The requirement's stale
pending-implementation label and temporary E2E timeout diagnostics were removed.

After reconciling main's shared-policy refactor, the affected backend race
selection, 65 frontend tests, typecheck, and all 16 desktop/phone browser tests
passed again. Four fresh captures replace the earlier verification assets.
The initial PR head's 89 passed, 15 skipped, and one neutral remote check are
historical; the remediation commit requires fresh remote CI and review evidence.
