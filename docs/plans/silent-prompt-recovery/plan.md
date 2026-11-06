---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/agents/system-design/agent-stall-recovery.md
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
legacy_specs: []
---

# Implementation plan: Silent prompt recovery

## Overview

Keep a silent accepted turn available for controlled cancellation. Let one
Resume request tolerate a short projection delay without weakening recovery
proof. Implement Task 01, then Task 02 in the primary session.
Both work orders are complete.

Agents owns inactivity classification and its visible notice. Platform owns
durable uncertainty, projection, and admission. Tasks consumes these contracts;
there is no separate UI requirement.

## Evidence and settled intent

The 19 October incident records prompt acceptance at 11:55:10 Lisbon time,
watchdog teardown at 12:00:10, then an unknown-outcome delivery block.
The first Resume found incomplete projection; the next succeeded.
The user approved advisory handling, controlled cancellation, a bounded Resume
wait, and diagnostic capture after the Codex investigation.

Local probes reproduced a default first timeout at 300.152 seconds and native
WebSocket retries with no ACP notification until fallback. Fresh and copied
history prompts succeeded. These results establish a possible silent-wait
mechanism, not the historical provider failure.

The detailed incident record remains in Kandev task
`8daab7f0-5b6f-4831-b7c8-b5feb10992b2`. Sanitized local experiments are under
`.kandev/diagnostics/codex-silence-2026-10-19/`; these ignored artifacts are
supporting evidence, not required test inputs.

## Scope

In scope: separate acceptance from output evidence; advisory no-output stalls;
confirmed or uncertain cancellation; bounded sanitized stall diagnostics;
bounded native-resume projection waiting; desktop and phone regression tests.

Excluded: changing the live affected task, timeout settings, automatic resend,
Codex upgrades, upstream fixes, native private-log collection, new recovery UI,
schema migrations, and unrelated Office scheduling changes.

## Technical approach

Use the existing `dispatchedPromptGeneration` acceptance boundary in lifecycle,
with generation-safe snapshots and authenticated restore evidence. Do not set
`agentEventSincePrompt` for acceptance: it also feeds output/effect evidence.
Keep metadata outside the progress clock.

`handleAgentStalled` reuses the neutral running notice for a silent accepted or
unconfirmed prompt. Trusted startup errors still use their existing failure
paths. The action remains `agent.cancel`, with current cancellation ownership,
busy projection, and durable uncertainty handling.

`GetNativeResumeDeliveryEvidence` gains a bounded wait only for verified
projection lag. Poll at 200 ms for at most 10 seconds within the caller's
deadline; recheck execution ownership and rebuild the full proof after catch-up.
Do not retry authentication, identity, storage, live-work, or malformed-evidence
errors. Keep SQL transactional resolution as the final gate.

### Compatibility

| Path | Evidence and intended behavior | Required proof |
| --- | --- | --- |
| Codex through ACP | Current agentctl acceptance; silence stays advisory | Mock delayed output plus cancellation; existing isolated probes |
| Other ACP agents | Same transport acceptance contract; no provider-specific timer | Table-driven lifecycle and handler tests |
| Durable adopted turn | Authenticated current submission, stream, incarnation and generation | Restore test; stale owner cannot mark acceptance |
| Legacy peer or absent acceptance | Unknown; metadata cannot prove start or failure | No synthetic terminal outcome or output evidence |
| Explicit startup error | Existing terminal path and owned cleanup | Existing launch-failure regressions |
| CLI passthrough | No managed prompt watchdog added | Existing exclusions retained |

## UI preview

The user requires Kandev rich output for previews, so this package uses named
state descriptions and a rich preview instead of ASCII drawings.

**UI-01: Silent turn**, task chat or Quick Chat, desktop and phone.
Before: a no-output timeout produces a settled error and removes cancellation.
After: the transcript contains “Still waiting for the agent.” followed by the
existing inline **Cancel turn** action. It stays neutral and content-width.
The conversation remains the scroll owner; no dialog or new fixed panel appears.
Phone and coarse-pointer targets are at least 44px; compact desktop styling
stays unchanged. This maps to AC-AGENTS-AGENT-STALL-RECOVERY-001.4-.6 and .12-.14.

**UI-02: Recovery**, same existing recovery surface.
During cancellation or Resume, existing busy state prevents duplicate actions.
Confirmed cancellation settles normally. An uncertain outcome retains the
existing blocked composer and appropriate recovery controls after reload.
During a short projection delay, Resume remains busy and completes from one
click. Timeout keeps the block and existing error feedback.
This maps to AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2, .9, .25 and .26.

The shipped `action-message-actions.tsx` inline action and
`task-layout.tsx` mobile chat are the presentation exemplars. Reuse the existing
localized text, semantic buttons, keyboard behavior, and shared state. No new
motion or decorative alert treatment is needed. Any necessary new copy must
include all seven locales and the normal Traditional Chinese generation.

## Tests

Task 01 adds `TestWaitForPromptDone_AcceptedSilentPromptIsAdvisory` and
acceptance/metadata/stale-generation cases in `stall_activity_test.go`.
Handler tests in `event_handlers_stall_test.go` prove no stop, failure, or
duplicate notice. Cancellation integration tests prove both terminal
acknowledgment and unresolved delivery with no resend. Snapshot tests assert
bounded fields, rune-aligned truncation, rejection of malformed UTF-8, and
exclusion of synthetic prompt/token secrets.
These cover Agents criteria .1-.6, .8-.9 and .12-.15.

Task 02 adds `native_resume_projection_wait_test.go` in lifecycle; existing
orchestrator recovery tests cover the caller. Tests cover catch-up, deadline,
cancellation, identity change, storage error, and fresh full-proof validation.
`TestNativeResumeProjectionWaitRejectsUnresolvedJournal` supplies authenticated
unresolved journal work and a lagging cursor, then asserts an immediate blocked
result with no wait or scheduled retry.
`TestNativeResumeProjectionWaitContinuousProgressExhaustsOriginalBudget` proves
that repeated same-owner progress respects the 200 ms retry cadence and original
deadline. Real SQL/journal integration
retains the unknown submission and releases only the matching block.
These cover Platform criteria .5, .9, .12, .25 and .26.

The named tests above were added and pass; see each work order's Results.

## E2E tests

Extend `session/stall-notice-recovery.spec.ts` and
`session/mobile-stall-notice-recovery.spec.ts`: accepted silent turn, notice,
Cancel, confirmed settlement, uncertain outcome, reload, and later activity.
Use a controlled mock and deterministic watchdog clock seam. Do not wait five
real minutes or satisfy the regression by seeding only a notice row.

Add `session/native-resume-projection.spec.ts` and
`session/mobile-native-resume-projection.spec.ts`: one Resume click while
projection is delayed; busy control; eventual input readiness; timeout stays
blocked; no old-prompt dispatch. Run Chromium and mobile-chrome sequentially.
Phone checks include a narrow fine-pointer viewport, 44px targets, containment,
and no horizontal overflow.

## Work orders

- [x] [Task 01: Preserve silent accepted turns](task-01-silent-turns.md)
- [x] [Task 02: Wait for resume projection](task-02-resume-projection.md)

Both work orders use TDD. Before the first pnpm command in this fresh worktree,
run `(cd apps && pnpm install --frozen-lockfile)`. Use isolated E2E instances;
do not attach to the user's main backend.

## Related delivery records

[Never-started stall teardown](../never-started-stall-teardown/plan.md) and
[stale stall notices](../stale-stall-notice/plan.md) remain historical records.
This package supersedes the inactivity-only terminal classification and forced
teardown.
It preserves the progress clock, stop reachability, and notice resolution.
[Journal shutdown recovery](../agentctl-journal-shutdown-recovery/plan.md)
retains its recovery and replay boundaries.

## Verification results

Design checks passed on 20 October 2026:

- `python3 scripts/list-docs.py validate`: 385 decisions and 1616 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: both work orders
  resolve their plan, designs, requirement IDs and acceptance criteria. A planned
  production path was supplied to exercise coverage for this docs-only diff.
- `git diff --check`: passed.

Both work orders are implemented and verified. The final recovery race suites
passed for lifecycle, orchestrator, and SQLite. Focused projection tests verify
cancellation, ownership changes, continuous progress, the fixed deadline,
client lease release, and real SQL/journal resolution. Changed-package Go lint,
TypeScript, ESLint, i18n, UI unit tests, and documentation checks passed.

Desktop and phone E2E cover silent turns, cancellation after reload, uncertainty,
Resume busy controls, success, and timeout/retry. Browser tests use an initialized
blocked session; backend integration controls actual projection lag. The final
SQL resolver remains fail-closed if evidence changes after the proof. See each
work order for the exact scope and verification results.

PR review corrections: clarify that forced teardown is superseded and include
the named accepted-silence regression in Task 01's test filter. A direct regex
check confirmed coverage. Catalog validation, all 36 linter tests, specification
lint, and diff whitespace checks passed after these documentation changes.
Follow-up review clarifications define valid UTF-8 truncation, stable full-proof
completion and paced retries within one deadline, and explicit unresolved-journal
and continuous-progress regression cases. Catalog validation, all 36 linter
tests, specification lint, and diff whitespace checks passed for this revision.

## Risks

- Acceptance is not output, effects, or proof that a remote model is healthy.
- Missing restored acceptance must not become permission to kill or resend.
- A wait under a projection or cancellation lock can deadlock recovery.
- A cursor becoming equal does not replace authenticated full-proof checks.
- Existing cancel escalation must not fabricate certainty for durable work.
- Diagnostics can retain only observed transport data; hidden native retries
  remain unavailable without an upstream protocol change.
