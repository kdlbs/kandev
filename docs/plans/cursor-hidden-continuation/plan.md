---
created: 2026-10-05
status: complete
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
  - ../../specs/platform/system-design/provider-error-recovery-cursor.md
  - ../../specs/platform/system-design/provider-interruption-continuation.md
legacy_specs: []
---

# Implementation Plan: Hidden Cursor continuation and truthful provider errors

## Overview

Correct Cursor diagnostic projection, then admit completed-tool continuation
through the existing retry owner, then prove native behavior and visible recovery.
The internal prompt is exactly `continue`, independent of the task subject and
absent from Kandev chat history. Keep one five-attempt backoff and the current
experimental rollout toggle.

Platform owns the shared recovery contract, with Agents providing ordered ACP
evidence and Tasks providing accepted-turn/message projection. Requirements are
the amended [provider error contract](../../specs/platform/requirements/provider-error-recovery.md)
and [continuation contract](../../specs/platform/requirements/provider-interruption-continuation.md).
The [decision](../../decisions/2026-10-05-hidden-completed-tool-continuation.md)
records the intentional change from read-only to successful-completion admission.

## Confirmed evidence and scope

The inspected October 5 session's final raw ACP notification was
`Error: RetriableError: [resource_exhausted] Error`. Both PR-read shell tools
reported `completed` before it. `adapter_prompt.go` replaced any matching marker
with `cursorRetriableStreamResetMessage`; the displayed HTTP/2 message was generated
by Kandev. Retained runtime state reported `refused`, zero started retries, and a
preserved native conversation. No upstream TCP failure was established.

Use a minimal sanitized synthetic fixture of those frame shapes. Do not copy the
live transcript, repository/PR names, provider tokens, or private tool output.
The smallest regression emits two successfully completed `execute` calls followed
by the exact resource-exhaustion envelope and normal prompt settlement.

### In scope

- Sanitized actual diagnostics and distinct retryable resource-exhaustion category.
- V2 successful-completion evidence for foreground tools of every category.
- Exact internal `continue`, no user-message insertion or prompt-history pollution.
- Existing runtime preservation/restoration, retry ownership, budget, cancellation,
  and native identity proof; desktop/phone recovery and reload proof.
- Experimental metadata, public recovery documentation, and native compatibility
  evidence required by the broader tool-outcome contract.

### Out of scope

- Original replay after tools; unknown/pending/failed tool recovery; provider or
  model switching; new retry timers/tables; jitter; automatic flag promotion.
- Dynamic/Office/utility/passthrough policy changes; generic data-only ACP error
  classification; arbitrary other-provider support; mutating the reported session.

## Technical approach

### Diagnostic projection

Retain sanitized bounded Cursor diagnostic/category in `promptTurnState`, clear
it on real provider progress, and export it after the existing notification barrier.
Add `provider_resource_exhausted` to `routingerr` with exact Cursor marker fixtures,
short same-provider policy, and runtime-continuity recognition. Keep hard error,
unknown, cancellation, and foreign-provider negatives. No raw-message branching
in orchestration or the frontend.

### Completion-based continuation

Add `native_saved_history_completed_tools_v2`; preserve V1 read-only interpretation.
Use an ordered bounded foreground completion ledger and immutable terminal snapshot.
`SafeFor` checks declared version, completeness, every successful outcome, pending
ownership, and current generation. Maintain original replay evidence independently.
Expand continuation cause admission through shared short-retry classification.

Change `continuationInstruction` to `continue`. Existing `internalContinuation`
already bypasses prompt augmentation and user caching; dispatch-only avoids message
records. Prove actual outgoing ACP payload, message/history projection, and accepted
turn identity. Update mock detection only for fixture-owned continuation episodes.

### Compatibility matrix

| Shape | Outcome | Proof/fallback |
| --- | --- | --- |
| Cursor V2, output plus successful shell/write/MCP tools | Same native conversation gets hidden `continue` | Native isolated probe and ordered fixtures |
| V1 or older remote helper | Existing limited recovery | No reinterpreted completion guarantee |
| One completed tool plus one pending/failed/unknown tool | Manual, zero dispatch | Mixed-state regression |
| Usable ACP runtime | Same process/connection, no restore | Retained-runtime integration |
| Lost runtime with strict native restore | Same saved native ID or refusal | Native restore integration |
| Unknown diagnostic, explicit hard quota/auth, cancellation | Existing manual policy | Catalogue collision tests |
| Other providers/dynamic/Office/utility/passthrough | Existing owner | Existing negative matrix |

### Rollout and documentation

Reuse `features.providerInterruptionContinuation` /
`KANDEV_FEATURES_PROVIDER_INTERRUPTION_CONTINUATION`, high risk, restart required,
off in prod/dev/e2e. Update its read-only metadata description; keep environment >
saved override > profile precedence. Enabled evidence/admission gates remain backend
authoritative. Categorization correction applies with the toggle off as well.
Document the new semantics in `docs/public/tasks-and-workflows.md` and the existing
feature-toggle/operator section in `docs/public/operations.md` during implementation.
Check `README.md` and `docs/screenshots.md` for stale claims without adding unrelated
content. No public documentation changes are needed in this design-only turn.

## ASCII UI preview

UI-01: Task Chat during recovery. Existing inline status; internal prompt omitted.

```text
Desktop Chat
  [User's original request]
  [Completed tool results]
  Cursor: Resource exhausted. Attempt 1/5. 5s       [Cancel]
  Cursor: Continuing...                           [Cancel]
  [Agent response completing the unfinished work]
  [Composer]

Phone Chat
  [User's original request]
  [Completed tool results]
  Cursor: Resource exhausted
  Attempt 1/5. Retrying in 5s
  [ Cancel (44px minimum) ]
  [Agent response after continuation]
  [Composer in existing safe area]
```

Copy is illustrative and localized. At most one backend-owned status is current;
waiting and continuing are successive states. There is no user `continue` bubble
or placeholder. Chat retains one transcript scroll owner; desktop controls keep
their existing density. Phone reuses `mobile-provider-interruption-continuation`
and its inline stacked action. Cover wrapping, keyboard/focus stability, existing
touch target, reload, pagination, and another viewer under AC `001.6`, `001.7`,
and `003.1` through `003.4`.

## Tests

| Contract | Required regression evidence |
| --- | --- |
| Provider-error `001.12`-`.15`, `.30`, `.31` | `TestCursorRetriableDiagnosticProjection`, `TestClassifyCursorRetriableCategory`, catalogue/policy collisions |
| Continuation `001.1`-`.5`, `.7` | `TestCursorContinuationEvidenceToolOutcomes`, V1/V2 wire skew, `TestInterruptionContinuationCompletedTools` |
| Continuation `001.6` | `TestInterruptionContinuationHiddenContinue`, exact wire payload, user-message list and prompt index, ordinary user `continue` |
| Continuation `002.1`-`.4` | Existing budget, queue/ownership, cancellation, retained-runtime, settlement, restart families with V2/resource fixtures |
| Continuation `002.5` | Config/registry/profile/frontend contracts and disabled evidence/dispatch negatives |
| Continuation `003.1`-`.4` | Desktop/phone E2E status, disclosure, Cancel, no internal prompt, persisted history |

All new tests use TDD. Preserve existing V1 negatives rather than globally changing
their expected result to V2. Native evidence is a prerequisite to completed V2
delivery and must not be replaced by a skipped or mock-only probe.

## E2E tests

Extend `tests/session/provider-interruption-continuation.spec.ts` and
`mobile-provider-interruption-continuation.spec.ts` using the existing isolated
fixture. Add resource-exhaustion after successful shell/write tools, repeated
recovery with a shared budget, mixed pending outcomes, hidden payload, and visible
human `continue`. Prove no automatic user message in DOM and API history after
reload/pagination and a second viewer. Assert actual selected native identity and
outgoing prompt trace, not only a helper-generated completion string.

## Work orders

- [x] [Task 01: Preserve and classify actual Cursor diagnostics](task-01-cursor-error-category.md)
- [x] [Task 02: Dispatch hidden continuation after completed tools](task-02-hidden-continue.md)
- [x] [Task 03: Prove native and desktop/mobile recovery](task-03-recovery-proof.md)

Execute sequentially. Task 02 depends on Task 01; Task 03 depends on Task 02.
Completed `provider-interruption-continuation`, `transient-turn-runtime-continuity`,
and `cursor-retriable-stream-reset` packages remain historical implementation
evidence. This package amends only the contracts named here; no historical counts
or done statuses are rewritten.

## Verification results

Implementation and review remediation verification:

- Routing catalogue, ACP adapter, wire snapshots, watcher, mock-agent, runtime
  registry, and profile packages pass with `go test -tags fts5 -race`.
- The full orchestrator race suite passes. Focused lifecycle continuation,
  transport-loss, and turn-failure tests pass under the race detector.
- The opt-in native Cursor matrix passes on CLI `2026.10.01-e373342`:
  session resume negotiation, saved-conversation restoration, abrupt disconnect,
  and completed edit/shell tools followed by exact `continue`.
- Native edit/shell probes interrupt only after ACP completion and before the
  final answer; same-ID restore and the original requested response are asserted.
  Cursor reports `loadSession=true` and does not support `session/resume` in
  this version. Saved tool output is not guaranteed to be recalled after restart;
  restored continuations may inspect current state or choose to repeat work.
- Desktop continuation E2E: 15 cases pass in the managed Docker runner,
  including successful shell/edit tools, pending/unknown refusal, literal hidden
  continuation, visible human `continue`, reload/second viewer, cancellation,
  human priority, and both backend-restart survival modes.
- Frontend feature, recovery feedback, and action-message tests: 63 pass.
  Typecheck, changed-file lint, and all-language i18n checks pass.
- Backend lint reports zero issues. Public documentation, catalog validation,
  specification lint, and whitespace validation pass.

The full lifecycle suite also exercises unrelated setup and SSH process tests
that cannot write the real user's global Git configuration or control protected
processes in this sandbox. Those failures reproduce on the unchanged base
commit. They are recorded separately from the passing affected recovery tests.
Desktop/phone resource-exhaustion presentation and final delivery screenshots
are verified separately using synthetic seeded status data; error projection
and recovery admission use the raw Cursor diagnostic regressions above.

## Risks

- Successful tool completion does not guarantee that a model will never choose
  to repeat an action. Host dispatch must never replay the original request.
- Literal `continue` depends on intact native history. Native proof must observe
  task completion and the same identity, not just accepted prompt delivery.
- New wire support must fail closed on older helpers/consumers; do not broaden V1.
- Recovery remains opt-in. Enabling it on the live installation or promoting it
  requires separate operator action; this package does neither.

Final delivery verification on the current main base: six focused desktop cases
and both phone cases pass in the managed Docker runner with retries disabled.
Fresh desktop and phone captures show the localized resource-exhaustion reason,
continuation countdown, and Cancel action. The phone target is at least 44px and
the document has no horizontal overflow. Captures use synthetic seeded status
data; raw diagnostic classification and admission are verified by backend tests.

PR review remediation narrows resource exhaustion to its verified complete
envelope, anchors unavailable/stalled categories, rejects the Cursor subagent
title alone, and refuses continuation after a prompt-gate handoff because late
permission/tool frames cannot prove originating ownership. Mock episodes are
consumed once. V1 positive admission/dispatch coverage remains; the lifecycle
fixture asserts a valid stored diagnostic. Reload checks wait for persisted
content, disabled-mode traces admit only the original prompt, and the seeded
resource-exhaustion test claims presentation coverage only. Specification
statuses and public foreground/uncertain-work boundaries are synchronized.
Remote CI and review disposition remain pending until the final fixup snapshot.

The cancellation E2E exposed a separate settlement bug: disabling workflow
advancement for an automatic continuation also disabled session parking.
Confirmed continuation cancellation now requires WAITING_FOR_INPUT independently
of workflow completion eligibility, under the existing captured-turn guard.
The focused regression reproduced RUNNING after cancellation before the fix.

Post-fixup local verification:

- `go test -tags fts5 -race ./internal/orchestrator` passes, including the new
  cancellation-settlement regression. Routing, ACP, mock-agent, and focused
  lifecycle race checks pass for the review remediation.
- Desktop continuation/resource presentation suite: 15 of 16 cases passed and
  the cancellation-settlement failure above was reproduced and fixed. After the
  fix, the three affected desktop cases (accepted continuation and cancellation,
  cancellation while waiting, and queued human priority) pass with retries
  disabled. Both phone continuation/resource presentation cases pass.
- CI backend shard 1 exposed `TestManagedDeletionHostReceipts/completed_replay`
  creating a replacement before asynchronous deletion cleanup finished. Pausing
  the fixture worker reproduces its APPLIED-versus-CONFLICT failure. The test now
  waits for the cleanup barrier to finish before replacement creation; no plugin
  production behavior or contract changes.

The cleanup-ordering regression passes for all receipt modes across 20 race
runs: `go test -tags fts5 -race ./internal/plugins -run
'^TestManagedDeletionHostReceipts$' -count=20`.

The full plugin package also passes CI's race/coverage settings:
`go test -race -covermode=atomic -coverprofile=<temporary-path> ./internal/plugins`
(68.0% statement coverage).
