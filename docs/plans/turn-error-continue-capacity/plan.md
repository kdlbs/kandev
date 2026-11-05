---
created: 2026-11-05
status: complete
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-003
  - REQ-PLATFORM-TURN-CONTINUITY-005
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
  - ../../specs/platform/system-design/acp-prompt-error-continuity.md
legacy_specs: []
---

# Implementation plan: Turn error continuation and capacity recovery

## Overview

Recover the reported capacity failure automatically, and combine manual dismissal with a normal continuation message.
Deliver capacity evidence and retry admission first. Then deliver the manual action using existing message acceptance and acknowledgement.
Both work orders are complete. Implementation ran after the later explicit implementation request.

Platform owns shared turn recovery and attention. Tasks supplies existing message admission, conversation records, and stamp comparison.
The [capacity requirements](../../specs/platform/requirements/transient-turn-runtime-continuity.md) and [manual continuity requirements](../../specs/platform/requirements/acp-prompt-error-continuity.md) remain the sources of truth.
Requirements 004 and 005 moved into the second document to keep both specifications within their size limits.
Their IDs and earlier acceptance criteria remain unchanged.

## Evidence and assumption check

Confirmed intent: one Dismiss and continue action, plus automatic exponential backoff for the reported model-capacity error.
No material product question remains after tracing the supplied session.
Default policy retains the existing eleven-attempt, thirty-minute capacity episode. No new setting or retry schedule is necessary.

Read-only evidence comes from `/root/.kandev/data/kandev.db` and the authorized native ACP log under `/root/.kandev/logs/acp/`.
The native session is `01a1abc1-5236-7423-9feb-65da88340874`.
The reported task is `cfcacb4c-580d-46ab-ba6a-3268a43925c6`, with session `14c2f700-1595-4366-ac87-7e2f978659fd`.
These are incident references, never reproduction targets.

- At 14:23:08 UTC, the turn contained 43 successful tools and one command with exit code 1.
- At 14:49:27 UTC, the pictured turn contained 31 successful tools and six commands with nonzero exit codes.
- Both turns have no pending tools in the captured native ledger. Both durable error records report zero started retries and `unsafe_work`.
- Native failed updates carry matching `terminal_exit.terminal_id`, integer exit codes, and `signal: null`.
- The screenshot's preceding service-regression update belongs to the second turn, at 14:42:09 UTC.

Native failed frames occur at lines 902, 1450, 1499, 1574, 1780, 1791, and 1797 of that session's raw log.
Keep only sanitized structural fixtures. Do not copy raw transcripts, command output, credentials, or account data into the repository.
The log proves observed terminal outcomes. It does not establish rollback or exactly-once command effects.

## Scope

### In scope

- Codex capacity continuation after confirmed ordinary command failures, with a versioned evidence contract.
- Existing backend backoff, cancellation, ownership, and original-prompt replay fences.
- A localized inline Dismiss and continue action in desktop, phone, preview Chat, and task-backed Quick Chat.
- One visible manual `continue` message, exact observed-stamp acknowledgement, duplicate suppression, and preserved drafts.
- Focused protocol, service, component, desktop, and phone evidence.
- Public recovery guidance when implementation lands.

### Out of scope

- Pending, cancelled, signalled, malformed, or uncertain tool outcomes and failed non-command tools.
- New failed-tool support for OpenCode or other providers.
- Original-prompt replay after work, model switching, native restoration after runtime loss, or new retry owners.
- Runtime flags, settings, schema changes, persistent retry jobs, and changes to autonomous task policies.
- Changes to the incident task, its process, or its database.

## Technical approach

### Capacity evidence and admission

Implement [the decision](../../decisions/2026-11-05-capacity-known-command-failures.md) in the current prompt ledger and snapshot.
Add proposed `CapacityContinuationCodexLiveSessionV2`, test-only mock V2, and `KnownFailedShellTools`.
Keep V1 predicates unchanged. Capture terminal identity, integer exit, signal, and cancellation evidence before synthetic cleanup.
Blocking failures still set `FailedTools`; eligible V2 command failures have their own count and retain their failed history.

Update `supportsCapacityContinuation`, admission, refusal, and successor predicates coherently in `provider_interruption_continuation.go`.
Keep `capacityContinuationHasUnsafeWork` aligned with the versioned snapshot predicate.
Do not broaden `ContinuationSafetySnapshot` or transport-loss recovery.
`handleTransientFailure`, `retryRetainedRuntimeContinuation`, and `provider_capacity_retry_policy.go` remain the sole scheduler and dispatch owners.
Keep the capacity instruction and eleven-delay ladder unchanged.

| Shape | Transport and identity | Intended behavior | Evidence and fallback |
| --- | --- | --- | --- |
| Codex V2, terminal nonzero execute tool, no other blockers | Settled ACP prompt, same execution/generation/native ID | Existing live capacity continuation and backoff | Native structural fixture, real SDK wire test, service and browser proof |
| Codex V1 or OpenCode V1 with a failed tool | Existing live-session support | Existing refusal | Legacy predicate tests. No V2 inference |
| Supported V1/V2 with successful tools only | Existing retained ACP binding | Existing continuation | Existing positive tests |
| V2 plus pending, signal, unknown, permission, or background evidence | Current prompt but blocked outcome ledger | Manual recovery | Negative adapter and orchestrator cases |
| Missing/unknown support or old remote component | Unproven evidence | Existing conservative handling | JSON round-trip and omission tests |
| Runtime replacement, lost native ID, changed authorization/configuration | Binding no longer valid | No delayed dispatch | Existing owner, cancellation, and runtime-loss barriers |
| Dynamic, Office, automation, utility, passthrough, other transport | Another owner or unsupported identity | Existing policy | No new capability from provider prose |

### Manual continuation

Extend the existing current-error controls in `SettledFailureMessage`.
Use a proposed `hooks/use-continue-turn-error.ts` hook, existing `sendMessageRequest`, and the displayed same-session error stamp.
Derive a stable client message ID from that stamp and source session, within the existing 128-byte limit.
Use exact `continue` as manual content. Keep current applied configuration and leave unsent composer state untouched.
Backend `message.add` already persists and dispatches the message and acknowledges the exact observed stamp after acceptance.
Do not call Dismiss first, add an endpoint, or create another automatic loop.

Use existing ID reconciliation and replay fingerprints for lost responses and multiple viewers.
On acknowledgement failure, keep Dismiss available and do not resubmit accepted work.
Current session admission selectors suppress continuation during automatic recovery, another turn, cancellation, permission barriers, or runtime recovery.
A race can still reach ordinary backend admission, including normal capacity queueing. Report the accepted outcome accurately.

## UI preview

These structured views specify control order and state. Labels are illustrative English localization sources.
Kandev rich output supplies the conversation preview. Spacing is not a pixel specification.

| View and entry | State | Hierarchy and actions |
| --- | --- | --- |
| UI-01 desktop, current error in Chat | Manual continuation available | Provider error, review guidance, primary Dismiss and continue, secondary Dismiss turn error, technical details. Both controls share one compact row |
| UI-01 phone, current error in Chat | Same manual state | Provider error and guidance wrap. Primary Dismiss and continue occupies the first full-width row. Dismiss turn error occupies the next row. Details follow |
| UI-02 desktop and phone | Submission pending | Keep stable action labels. Disable both competing controls. Show one localized Continuing status without moving focus |
| UI-03 desktop and phone | Accepted or acknowledgement failed | Keep historical error and visible manual continue message. Successful acknowledgement removes active controls. Failed acknowledgement keeps attention and Dismiss only |
| UI-04 desktop and phone | Capacity retry waiting | Reuse the current notice with actual attempt, countdown, recovery deadline, and Cancel. Do not offer a competing Dismiss and continue action |

The nearest curated phone layout is `mobile/session-mobile-layout.tsx`.
The shipped error-control exemplar is `messages/turn-error-dismiss-button.tsx`.
This short action stays inline because it needs transcript context and no navigation or extra choices.
The transcript retains one scroll owner. Composer safe areas, keyboard behavior, attachments, and drafts retain their existing ownership.
Fine-pointer desktop controls measure 28px within 1px. Phone and coarse-pointer targets measure at least 44px.
Phone rows remain stacked at narrow fine-pointer widths too. Long translated labels wrap without horizontal document overflow.
Screen readers announce phase changes through one status region. Countdown ticks do not become announcements.

Mappings: UI-01 through UI-03 cover AC-PLATFORM-TURN-CONTINUITY-005.9 through 005.12.
UI-04 covers AC-PLATFORM-TURN-CONTINUITY-003.2 through 003.5 and 003.10.

## Tests

Proposed tests use the following names. Existing positive, negative, and timing selections remain part of each work order.

| Acceptance | Evidence |
| --- | --- |
| 003.1, 003.6, 003.10 | `capacity_continuation_test.go`: `TestCapacityContinuationV2KnownFailedShellTools`. New adapter `adapter_capacity_known_command_test.go`: `TestCodexCapacityKnownCommandFailure` |
| 003.2, 003.4, 003.8, 003.10 | New orchestrator `provider_interruption_known_command_test.go`: `TestCapacityContinuationKnownCommandFailure`, including mixed blockers, cancellation, and binding changes |
| 003.3, 003.5, 003.7 | Existing `provider_interruption_capacity_delayed_test.go`, extended with a known-failure V2 episode, budget transfer, expiry, and restart retirement |
| 003.6, 003.10 | Existing snapshot round-trip tests in process, lifecycle, and watcher packages, extended for V2 counts and old/unknown support |
| 005.9, 005.10, 005.12 | New `hooks/use-continue-turn-error.test.tsx` and `action-message-turn-error-continue.test.tsx` cases for current/sibling/historical errors, admission barriers, applied configuration, and untouched drafts |
| 005.11 | New hook tests for rapid activation, response loss, accepted acknowledgement failure, and stable IDs across remounts. Extend `message_handlers_turn_error_test.go` for same-ID acceptance and stale/newer attention |

## E2E tests

Task 01 extends `transient-turn-runtime-continuity.spec.ts` and its `mobile-` counterpart.
Prove a real mock ACP turn with a confirmed failed command enters backoff and accepts one continuation on the same runtime.
Use `capacity-retry-clock.ts` to observe scheduled waiting and advance delays without real-time sleeps.
Assert the actual failed result survives, the original request occurs once, and a side effect is not repeated.
Cover repeated capacity failure, Cancel, reload, another viewer, pending-tool refusal, and unchanged V1 policy.
Use process, connection, native session, execution, and initialization evidence. A duplicate boot row alone proves nothing.

Task 02 extends `acp-prompt-error-continuity.spec.ts` and `mobile-acp-prompt-error-continuity.spec.ts`.
Click or tap the new action, observe a new accepted prompt and attributed agent response, and verify exactly one visible user `continue` message.
Keep draft text and attachment state before and after the action. Reload and verify durable attention and history.
Cover a rejected request, a lost response, rapid activation, a second viewer, newer sibling attention, preview Chat, and task-backed Quick Chat.
Measure desktop and phone targets and containment. Include a narrow fine-pointer phone layout and long localized labels.
Arm causal response/event observation before actions. Use existing fixtures and cleanup, and rebuild through the managed runner.

## Work orders

- [x] [Task 01: Retry capacity after confirmed command failures](task-01-known-command-capacity.md)
- [x] [Task 02: Dismiss and continue the current error](task-02-dismiss-and-continue.md)

Execute Task 01, then Task 02. Task 02 follows because both work orders update the same public recovery guidance and final browser scenarios.
Execution remains sequential in this primary session. A work-order wave does not authorize delegation.

## Verification results

Design-package validation passed:

- `python3 scripts/list-docs.py validate`: 432 decisions and 1921 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation `validateCoverage` preflight with planned runtime paths: covered, with no reference errors.
- `git diff --check -- docs/plans docs/specs docs/decisions`: passed.
- `git status --short -- docs/plans/turn-error-continue-capacity`: confirmed all three package files remain uncommitted.

The coverage preflight validated both new work orders and the mechanically updated historical requirement link at the design handoff.
It did not claim production files changed or product checks passed at that time.
Implementation, unit tests, browser tests, rendered comparisons, and public documentation updates are now complete.
Earlier completed packages remain historical evidence. They do not count as validation of this amendment.

Implementation verification passed:

- Backend protocol, lifecycle, watcher, orchestrator, mock-agent, and message-handler selections passed under `-race`.
- `bash scripts/lint-go-changed` reported 0 issues.
- The focused web hook/component selection passed 123 tests; `pnpm run typecheck` and Oxlint passed.
- The Traditional Chinese generator, pseudo generator, `i18n:check`, and `i18n:ratchet` passed.
- Desktop and phone capacity suites passed, including the new known-failed-command scenario and the unchanged eleven-attempt notice.
- Desktop and phone prompt-error suites passed, including preview Chat, task-backed Quick Chat, reload, a second viewer, and the narrow fine-pointer pseudo-locale layout.
- Public-doc validation and all-spec lint passed.

## Risks

- A nonzero exit can leave partial effects. Keep continuation and original-prompt replay distinct.
- Terminal parsing must reject signals, mismatched identities, and contradictory updates rather than converting them into known outcomes.
- A new support token must fail closed on older hosts and leave OpenCode V1 unchanged.
- Reusing a random ID per click or clearing attention before message acceptance can duplicate work or lose the alert.
- Unsent composer changes must not become hidden attachments or mode changes in the continuation.
- Capacity refusal still needs manual recovery for pending or uncertain work. This package does not promise every capacity error will retry.
