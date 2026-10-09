---
id: "02-remove-parking-presentation"
title: "Remove parking presentation and verify user flows"
status: completed
wave: 2
depends_on: ["01-repair-session-open-eligibility"]
plan: "plan.md"
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003
acceptance_criteria:
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.2
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.3
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.5
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.6
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.7
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.8
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.9
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.4
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.6
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.10
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
---

# Task 02: Remove parking presentation and verify user flows

## Summary

Remove the parked-session note from shared chat surfaces. Verify selected
conversation recovery on desktop and phone without adding another parking affordance.

## In scope

- Remove `ParkedSessionNote`, `hasWorkflowParkingMarker`, and their chat-panel use.
- Remove `parkedSessionNote` from en, pseudo, pt-pt, zh-cn, zh-hk, zh-tw, and ja catalogs.
- Replace parked-note tests with absence and ordinary-conversation-control assertions.
- Update desktop/mobile queued-session E2E scenarios according to the plan matrix.
- Verify restart, capacity, prevention preference, conversation identity, and no prompt replay.
- Capture `session_open` recovery on desktop and phone when capacity permits.
- Cover the confirmed Review-to-Implement peer resume described in the plan.
- Preserve legacy metadata compatibility without a live-data rewrite.
- Update both public recovery explanations and affected companion-plan references.

## Out of scope

No replacement parking toolbar, badge, tooltip, or composer restriction. Genuine
queue status remains. No new layout, navigation hierarchy, or automatic prompt.

## Acceptance

1. No parking presentation appears on desktop or phone, including with persisted legacy markers.
2. Opening the selected stopped session resumes it when normal controls permit,
   without clicking Resume. Queue ownership and workflow primary remain unchanged.
3. The reviewer messages the implementation session exactly once through MCP. Active output, reload, and turn settlement show no parking UI.
   Review ownership stays unchanged.
4. Selecting a stopped session launches normal `session_open` recovery when capacity permits,
   without replaying its workflow prompt or changing Review ownership.
5. Both E2E projects and targeted UI/i18n checks pass. Public docs describe the new behavior.

## ASCII UI preview

Use [UI-01 and UI-02](plan.md#ascii-ui-preview).

```text
Desktop: [Session tabs] -> Conversation -> Composer
Phone:   [Session picker] -> Conversation -> Composer -> Bottom navigation
Both:    No parked-session row; genuine task queue status remains when needed.
```

Keep the existing desktop tabs and phone task drawer/session picker.
The chat remains the single conversation scroll owner. No new touch targets or
hover-only behavior are introduced. Cover AC 001.9 and 003.10.

## Regression implementation

Follow [the peer-message scenario](plan.md#peer-message-handoff-regression).
The existing `cmd/mock-agent/script.go:executeMCPCommand` supports the real MCP call.
The mock-agent fixture must retain the MCP server definitions supplied to `LoadSession`
so the resumed Review session can issue that call. Do not change the application MCP path.

Add the named desktop and phone regression tests before banner removal.
Record a RED result caused by the visible parked note during active output.
Then remove the component, marker predicate, imports, obsolete tests, and locale key.
Retain queue component tests. Use rendered chat/E2E absence checks instead of tests
that only assert a removed export or file no longer exists.

Confirm the no-banner result with stored legacy markers. Cover STARTING, RUNNING,
and WAITING_FOR_INPUT where the fixture exposes those states. The fix removes
parking presentation in every state, rather than changing the state predicate.
Do not delete stop-intent tombstones or add a parking-clear requirement to peer delivery.

The nearest phone exemplar is `components/task/mobile/session-task-switcher-sheet.tsx`.
Use `mobile-sessions-pill` and `mobile-session-row-<id>` from the existing mobile E2E.
Keep the dedicated phone layout, chat scroll owner, safe-area navigation, and composer.
Assert no document horizontal overflow and a session-row hit target of at least 44px.

## Verification

Run from repository root; install dependencies once for a fresh worktree.

```bash
(cd apps/backend && go test ./cmd/mock-agent -run 'TestLoadSession(RetainsMCPServersForResumedPrompt|ReturnsCapabilitiesForResumedSession)' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/launch-queue-status.test.tsx hooks/domains/session/use-session-resumption.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/queued-session-ownership.spec.ts tests/workflow/workflow-peer-resume.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-queued-session-ownership.spec.ts tests/workflow/mobile-workflow-peer-resume.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run E2E projects sequentially with the managed runner. Use only disposable fixture
backends and scoped settings. Record behavioral RED before the UI removal and
GREEN after it. Backend Task 01 owns recovery RED; these tests prove integration.
Assert provider readiness independently of workspace-only readiness.

## Files likely touched

- `apps/web/components/task/launch-queue-status.tsx` and its test.
- `apps/web/components/task/task-chat-panel.tsx`.
- `apps/web/hooks/domains/session/use-session-resumption.test.ts`.
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw,ja}/task.json`.
- `apps/web/e2e/tests/workflow/queued-session-ownership.spec.ts`.
- `apps/web/e2e/tests/workflow/mobile-queued-session-ownership.spec.ts`.
- `apps/web/e2e/tests/workflow/queued-session-ownership-helpers.ts`.
- `apps/web/e2e/tests/workflow/workflow-peer-resume.spec.ts` (new).
- `apps/web/e2e/tests/workflow/mobile-workflow-peer-resume.spec.ts` (new).
- `apps/web/e2e/tests/workflow/workflow-peer-resume-helpers.ts` (new).
- `apps/backend/cmd/mock-agent/main.go` and `session_config_test.go` for resumed MCP fixture support.
- `docs/public/tasks-and-workflows.md` and `docs/public/agents-and-profiles.md`.
- This package's status/results and companion references.

## Dependencies

Task 01. It supplies the backend behavior required by the rendered tests.

## Risks

A hidden note is insufficient if recovery still fails. Do not remove the genuine
queue region or confuse workspace restoration with a resumed provider session.
Restore fixture capacity and user settings even when assertions fail.

## Parallelism

`sequential`

## Inputs

- [Plan, previews, and E2E matrix](plan.md).
- [Design](../../specs/tasks/system-design/queued-session-ownership.md#conversation-recovery-and-workflow-stop-history).
- Existing phone queued-session E2E fixture and session page object.
- Mobile parity and E2E skills; public docs maintenance guidance.

## Results

Complete. The desktop peer-resume test first failed at the expected assertion:
the implementation session was RUNNING with active output and one parked note.
After presentation removal:

- Mock-agent LoadSession MCP retention test: passed.
- Targeted UI tests: 41 passed; typecheck and i18n check: passed.
- Chromium queued-session, peer-resume, and session-open E2E: 3 passed.
- Mobile queued-session, peer-resume, and session-open E2E: 3 passed.
- Public-doc validation: 62 tests and 47 published pages passed.
- Specification validation: 301 decisions, 1134 specifications, and 36 linter tests passed.
- `git diff --check`: passed.

The peer delivery occurred once. The active Implement session kept its legacy
parking marker while the banner stayed absent before and after reload and turn
settlement. Review remained the workflow step and the initial session remained
primary. The phone session row met the 44px target and showed no horizontal overflow.
The direct session-open flows captured `activation_source: session_open` and
confirmed the original workflow prompt was not replayed.
