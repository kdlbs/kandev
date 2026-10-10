---
id: "03-recovery-ui"
title: "Complete recovery controls and browser coverage"
status: completed
wave: 4
depends_on:
  - "02-session-recovery"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.6
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.17
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.18
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.19
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.5
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
  - ../../specs/platform/system-design/agent-runtime-availability.md
---

# Task 03: Complete recovery controls and browser coverage

## Summary

Show the actual retry outcome and its safe next action.
Align Retry connection and Stop, preserve touch targets, and prove post-crash recovery through the real browser path.

## In scope

- Consume Task 02's typed results in the recovery service, shared hook, and stopped-session view model.
- Announce pending, attached/settled, uncertain, unavailable-evidence, and blocked-ownership states with localized copy.
- Preserve request/session/recovery revision fencing and duplicate-action protection.
- Present eligible explicit native continuation inline with a new instruction and acknowledgment.
- Add Resume interrupted sessions to the runtime recovery notice, with selection, per-session reasons, and persistent batch results on desktop and phone.
- Distinguish restored-but-blocked from accepted continuation. Never report that work resumed based only on a successful native load or retry response.
- Keep Stop reachable during reconciliation. Show failures and unconfirmed cancellation without false success.
- Show delivery-storage pressure separately from process termination. Retry must report verified backlog cleanup or a specific blocked outcome.
- Make one action row own spacing. Render both buttons in that row; place status, warnings, and disclosures outside it.
- Reuse `controlSizingClassName` for both actions. Preserve existing recovery-card variants and focus behavior.
- Add all seven locale values. Generate Traditional Chinese with `pnpm run i18n:zh-hant`.
- Update recovery guidance in the `docs/public/sessions-and-review.md` during implementation.

## Out of scope

Global control redesign, additional dialogs, desktop layout preference changes, and live-instance testing.

## Acceptance

1. Every retry visibly reports a useful result. Eligible continuation dispatches only its new instruction; blocked ownership offers no unsafe resume.
2. Desktop Retry and Stop have equal top/bottom bounds within 1px, including pending and error states. Phone and coarse-pointer hit targets are at least 44px.
3. Real isolated child-crash flows pass on desktop and phone. Reload preserves uncertainty and available actions without duplicate messages or prompts.
4. Batch recovery preserves existing session and native conversation IDs, reports partial failures, and does not repeat accepted instructions after retry or refresh.

## ASCII UI preview

UI-01 and UI-02, excerpt from the [complete preview](plan.md#ascii-ui-preview).

```text
Desktop, uncertain:
[Retry connection] [Stop]
Checking the previous session...

Phone, uncertain:
[       Retry connection       ]
[             Stop            ]
Checking the previous session...

Desktop, eligible continuation:
Next instruction: [____________________________]
[ ] I reviewed the interruption and want to continue.
[Resume session] [Cancel]

Phone, eligible continuation:
Next instruction:
[_____________________________]
[ ] I reviewed the interruption
    and want to continue.
[        Resume session       ]
[            Cancel           ]
```

The labels are illustrative. The hierarchy, shared button row, separate status, and explicit continuation are required.
UI-04 adds a storage-pressure state in the same card:

```text
Desktop:
Output delivery is paused while saved output is synchronized.
[Retry connection] [Stop]

Phone:
Output delivery is paused while
saved output is synchronized.
[       Retry connection       ]
[             Stop            ]
```

Show that pause copy only when producer flow is actually paused. Otherwise report cancellation pending or delivery interrupted, as returned by the backend.
This view covers delivery criterion 001.4 and uses the same control sizing, keyboard behavior, and scroll owner.
The phone recovery notice uses the existing chat footer allocation so the fixed task header cannot intercept its controls.
The allocation bounds tall recovery content and retains its existing internal scroll behavior.
Phone controls stack below 768px; no fixed surface changes safe-area handling.
Use the existing recovery card and mobile runtime-replacement test as the exemplar.
Criteria: delivery 006.7-006.9 and runtime availability 003.5.

## Verification

Run from the repository root. Install dependencies once if this worktree has no installation.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/session-stopped-banner.test.tsx lib/session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-actions.test.ts lib/services/session-recovery-service.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/layout/agent-runtime-replacement.spec.ts tests/session/durable-stream-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-agent-runtime-replacement.spec.ts tests/session/mobile-durable-stream-recovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E runner rebuilds and owns isolated instances. Run the two browser commands sequentially.
Extend selected component files so every changed recovery branch is covered by these commands.
Proposed browser cases include missing-execution retry, retained terminal replay, explicit new instruction, blocked ownership, and Stop failure.
Assert prompt counts and identity, not only banner visibility.
Add an isolated full-journal/replacement test with SQL projection ahead of remote ACK. Verify cleanup and new output without a new conversation or duplicate prompt.
Exercise Retry and Stop during pressure on desktop and phone; a live harness must not be replaced by focusing the task.
Measure actual buttons on desktop fine pointer, phone, narrow fine pointer, and coarse-pointer tablet.
Include 767px/768px boundary checks, long localized labels, pending/error text, and no horizontal overflow.
Do not use class-string assertions as geometry evidence.

## Files likely touched

- `apps/web/components/task/chat/session-stopped-banner.tsx`
- `apps/web/components/task/recovery-actions.tsx`
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/lib/services/session-recovery-service.ts`
- `apps/web/lib/session-recovery-actions.ts`
- Adjacent component/service/hook tests and `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/layout/agent-runtime-replacement.spec.ts`
- `apps/web/e2e/tests/layout/mobile-agent-runtime-replacement.spec.ts`
- Existing durable stream-recovery E2E files and shared fixtures, only where the scenario needs them
- `docs/public/sessions-and-review.md`

## Dependencies

Task 02 supplies authoritative outcomes and continuation authorization.

## Risks

A shared component change can shift other recovery surfaces. Loading or disclosure content must not reintroduce wrapper-based alignment.
A new session in the same task preserves files but is not native conversation recovery.

## Parallelism

`sequential`

## Inputs

- [Persisted recovery presentation](../../specs/platform/system-design/durable-agent-reattachment.md#persisted-recovery-presentation).
- Mobile-parity control sizing and existing runtime-replacement E2E patterns.

## Results

Completed on 2026-10-09.

- The four required component, hook, and service suites passed. The final combined run added continuation, batch, revision-fence, delivery-view-model, and fixture freshness suites: nine files, 129 tests passed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and the staged `pnpm run i18n:ratchet` passed. All seven locales include the new copy.
- Both exact browser selections passed through `pnpm e2e:run --host`, sequentially with one worker. Chromium passed two specs in 33.9 seconds; mobile-chrome passed two specs in 30.2 seconds.
- Real child termination and replacement precede the full-journal fixture. It fills the unchanged 256 MiB quota with retained events already projected to SQL. Retry verifies zero retained bytes and ACK equal to the projected cursor before explicit continuation.
- Browser assertions preserve the Kandev and native conversation IDs, verify one canonical new instruction, show new output, and retain mixed batch results through retry and reload. Retained terminal projection and lifecycle-callback suppression are covered by the backend race regressions.
- Geometry checks cover 767px/768px, desktop fine pointer, phone, and coarse-pointer tablet. Pending and failed Stop states remain usable; long pseudo-localized copy has no horizontal overflow.
- Retry refreshes authoritative chat history. The phone notice uses the existing bounded footer allocation, keeping its controls clear of the fixed task header.
- The fixture is compiled by the managed runner and included in CI build identity and Docker artifacts. Browser execution does not need Go. Its source freshness regression passed after failing against the previous guard.
- Fresh desktop and phone screenshots were captured for explicit continuation and persistent results. Public recovery documentation was updated.

The browser full-journal scenario uses a confirmed stopped owner. Live producer pause, bounded cancellation, responsive control routes, and refusal to replace a live owner are covered by backend regressions; they are not claimed as live-provider browser evidence.

PR review follow-up:

- Browser checkpoints, pending requests, and batch results now belong to the full interruption identity. A later interruption resets the instruction and acknowledgment. Late preflight and dispatch responses cannot replace a newer checkpoint. Service, hook, and component regressions passed.
- The Resume action keeps its accessible name while a separate status announces progress. The component regression covers pending state, interruption replacement, and a late response.
- The final combined frontend run passed 147 tests across ten files. Type checking, full web lint, changed-service lint, and translation validation passed.
- The final browser selections passed sequentially: two Chromium specs in 39.5 seconds and two mobile-chrome specs in 28.8 seconds. Four fresh screenshots were captured and inspected. The staged translation ratchet and documentation coverage preflight passed.

## Superseding restart behavior

The user rejected the manual interruption flow on 2026-10-09.
[Task 05](task-05-silent-restart-recovery.md) replaces it with automatic restoration without prompt dispatch.
The results above describe the prior implementation, not validation of that revision.
