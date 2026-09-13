---
created: 2026-09-13
status: completed
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
system_design:
  - ../../specs/tasks/system-design/remote-contribution-tasks.md
  - ../../specs/agents/system-design/session-recovery-failures.md
legacy_specs: []
---

# Implementation Plan: Startup Recovery Scrolling and Timeout

## Overview

Allow more time for contribution checks on a busy machine. Keep the recovery
card and transcript in one scroll area. Implement two sequential work orders.

## Evidence and scope

Task `539ebf53-b1a8-476c-829a-f8b466287d34` failed during resume on September 13.
The preflight HTTP request exceeded its 30-second deadline at 17:09:06 Lisbon time.
A later preflight completed in 18.749 seconds. Resume succeeded at 17:09:52.
The user reported 100% CPU usage on the host. Logs do not independently prove
which resource delayed Git. The two-minute budget is a proposed tolerance,
not a measured guarantee under arbitrary host starvation.

The screenshot shows two scrollbars. Source confirms that `PanelBody` uses
`overflow-auto`, while its full-height transcript has another scroll viewport.
The recovery card adds height above that full-height child.

In scope: bounded preflight tolerance, transport deadline alignment, shared
chat scroll ownership, recovery reveal, and desktop/phone regression coverage.
Out of scope: global timeout increases, adaptive CPU policy, automatic retries,
Git publication, recovery authorization changes, and provider session replacement.

## Requirement conformance

Existing recovery criterion 006.6 and its responsive design require shared
scroll ownership. Draft criterion 006.7 defines reveal and user-scroll behavior.
Draft contribution criterion 002.6 defines the longer bounded check.
Existing contribution criteria 002.3 and 002.4 retain rejection and all-repository gates.
The owning requirements and designs contain both amendments.

## Technical approach

Task 01 gives the full contribution preflight loop two minutes. A dedicated
bounded request path avoids the ordinary HTTP client's 60-second cutoff.
Preserve earlier deadlines, cancellation, response decoding, and shared transport.
The existing overall readiness default is 15 minutes and needs no increase.

Task 02 moves the recovery card inside the transcript viewport and disables
outer scrolling in the chat panel. Coordinate one-time reveal with the existing
scroll controller. Key bootstrap reveals by session and failure stamp, and
fallback recovery reveals by session, attempt, and outcome. Keep an active
recovery reveal at the transcript top while older pages settle, then return
prepend ownership to the reader after a downward gesture.
No new recovery state store or duplicate action owner is necessary.

## ASCII UI preview

Before: a scrollable body contains a recovery card plus a full-height scrolling
transcript. Their combined height exceeds the available body height.

```text
UI-01: Task Chat, active startup failure (desktop and phone)
+---------------------------------------+
| Session navigation                    | fixed
+---------------------------------------+
| Startup failure summary             ^ |
| Resume / Restore / Start fresh      | |
| Recovery details (expand inline)    | | one scroll area
|                                     | |
| Conversation history                v |
+---------------------------------------+
| Composer                              | fixed
+---------------------------------------+
```

Desktop uses the existing compact action row. Phone stacks the same actions
with 44-pixel touch targets. Expanded details wrap in the same scroll area.
A new failure reveals the card once. The user can then scroll freely.
The composer retains safe-area clearance. Labels and spacing are illustrative.
Scroll ownership, action reachability, and one-time reveal are requirements.

## Mobile design contract

Entry: the dedicated task Chat surface in `mobile/session-mobile-layout.tsx`.
Exemplar: the existing inline recovery card and task mobile layout.
Hierarchy: cause, primary Resume action, secondary recovery actions, then details.
The card stays inline because it explains the current conversation failure.
Desktop and phone share recovery state and handlers. Phone uses stacked actions.
One transcript viewport owns vertical scrolling, including long expanded details.
No horizontal document overflow or new fixed-height phone wrapper is permitted.

## Tests

Task 01 covers contribution criteria 002.3, 002.4, and 002.6 with lifecycle and
agentctl client tests. Use controlled transport responses and short test budgets.
Prove extended-budget success, timeout, cancellation, earlier caller deadline,
and a mixed multi-repository result. No sustained CPU load is required.

## E2E tests

Task 02 extends `launch-failure-recovery.spec.ts` and
`mobile-launch-failure-recovery.spec.ts` for recovery criteria 006.1, 006.6, and 006.7.
Seed history taller than the viewport. Assert one scroll owner, initial/new-error
reveal, expanded details, user-controlled scrolling, reload, and composer access.
Check desktop, the configured phone project, and a short viewport.
Existing completed recovery packages retain their historical results.

## Work orders

- [x] [Task 01: Extend the bounded preflight budget](task-01-preflight-budget.md)
- [x] [Task 02: Use one recovery and transcript scroll area](task-02-chat-scroll.md)

## Verification results

Targeted product tests and review regressions: passed.

- `python3 scripts/list-docs.py validate`: passed (267 decisions, 868 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `(cd apps/backend && go test -race ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle)`: passed.
- `(cd apps/web && pnpm exec vitest run components/task/chat/message-list-native.test.tsx components/task/chat/message-list-native-scroll.test.ts components/quick-chat/quick-chat-session-view.test.tsx hooks/domains/session/use-session-resumption.test.ts)`: passed (110 tests), including controlled older-page settling, an in-flight page when a new failure arrives, and same-session fallback recovery attempts.
- `(cd apps/web && pnpm run lint && pnpm run typecheck && pnpm run i18n:check)`: passed.
- `(cd apps/backend && make build)`: passed.
- `(cd apps/backend && make test)`: reached unrelated environment-sensitive failures in process probes, config-home discovery, launcher, and Office SQLite migration tests; the changed `agentctl` and lifecycle packages passed.
- `(cd apps/web && pnpm run build)`: passed.
- Desktop and phone bootstrap recovery E2E scenarios: passed.
- `git diff --check`: passed.
- Plan status inventory: both work orders completed.

## Risks

A hung preflight can take longer to report failure. Cancellation must still work.
A new card reveal can conflict with bottom-follow or hidden-panel restoration.
Short viewports require the full card to share the transcript scroll area.
