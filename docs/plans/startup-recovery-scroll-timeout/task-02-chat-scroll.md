---
id: "02-chat-scroll"
title: "Use one recovery and transcript scroll area"
status: done
wave: 2
depends_on:
  - "01-preflight-budget"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.6
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.7
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 02: Use One Recovery and Transcript Scroll Area

## Summary

Put the recovery card inside the transcript viewport. Reveal a new failure once,
then preserve user scrolling and the existing composer layout.

## In scope

- Remove outer chat scrolling without changing the shared PanelBody default.
- Pass the recovery content into the real MessageList scroll viewport.
- Coordinate failure reveal, bottom-follow, session switching, and panel visibility.
- Cover desktop and phone, including long history and expanded details.
- Audit preview and Quick Chat callers for the same composition defect.

## Out of scope

New recovery actions, duplicate banners, translation changes, and transcript virtualization.
No permanent fixed recovery panel or nested details scroller.

## Acceptance

- Exactly one chat content viewport scrolls when history exceeds its height.
- Initial and new failures reveal the card once. Repeated state events preserve user position.
- Expanded details, recovery actions, and composer remain reachable on desktop and phone.

## ASCII UI preview

[Full preview and mobile contract](plan.md#ascii-ui-preview).
Applies to recovery criteria 006.1, 006.6, and 006.7.

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

## Verification

Extend the existing bootstrap-card scenarios with history taller than the viewport.
The expected RED is two independently scrollable ancestors or a card outside the
active viewport. Do not use a missing selector as the behavioral failure.

Assert computed overflow and actual scroll movement. Scroll history manually and
send the same error stamp again. Assert that the viewport does not jump.
Send a new stamp and assert that the card enters the viewport.
Expand long details and reach the final recovery action at a short height.
Keep the normal no-error bottom-follow case. Capture desktop and phone screenshots.
Use the existing isolated fixtures. Never navigate a test browser to the live task.

If dependencies are absent, run the install command once before package commands.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/task/launch-failure-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-launch-failure-recovery.spec.ts)
(cd apps/web && pnpm run typecheck)
git diff --check
```

If scroll-controller logic changes, add focused controller tests in the new
`message-list-native-scroll.test.ts` file. Run the exact command from the root:

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/message-list-native-scroll.test.ts)
```

## Files likely touched

- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native-scroll.test.ts`
- `apps/web/components/quick-chat/quick-chat-session-view.tsx` (if the same defect applies)
- `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts`
- `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts`
- A shared E2E helper beside these specs, if both need the same geometry checks.

## Dependencies

Task 01 precedes this task for sequential delivery. There is no schema dependency.

## Risks

Reveal can conflict with initial bottom positioning or hidden-panel restoration.
Use session and error identity, not translated error text, for reveal ownership.

## Parallelism

`sequential`

## Inputs

- [Recovery requirements](../../specs/agents/requirements/session-recovery-failures.md), requirement 006.
- [Recovery design](../../specs/agents/system-design/session-recovery-failures.md), responsive and scrolling sections.
- The current native transcript controller and bootstrap recovery E2E scenarios.

## Results

Moved task and Quick Chat recovery content into the native transcript viewport,
disabled the outer chat body scroll, and coordinated one-time recovery reveals
with the existing placement and auto-scroll controllers. The reveal is keyed by
session and failure identity, preserves later user scrolling, and keeps the
existing phone action layout and composer allocation.

Added focused scroll-controller coverage plus long-history desktop and phone
recovery scenarios. Both browser scenarios passed with expanded details, one
scroll owner, same-stamp position preservation, new-stamp reveal, composer
visibility, and phone touch-target checks.

Review fixes:

- Recovery-owned prepend pages keep the newly revealed card at the transcript
  top while a page settles, including a page already in flight when a new
  failure arrives. A real downward wheel, key, pointer, or touch gesture gives
  scroll ownership back to the reader, after which normal message anchoring
  resumes.
- The reveal key now follows the recovery surface that is actually rendered,
  including task-wide errors while another session is selected and persisted
  session metadata fallbacks.
- The one-time recovery placement latch clears after the initial reveal even
  while feedback remains visible, so normal bottom-follow, activation catch-up,
  and persisted-offset restoration resume afterward.
- Quick Chat fallback recovery keys now use the session, monotonic recovery
  attempt, and outcome identity. Clearing feedback re-arms the reveal latch.
  Duplicate same-attempt state, including changed translated error text,
  keeps the existing key.
- Added controlled pagination, same-session recovery, visible-error-key, and
  post-reveal auto-scroll regression coverage. The review cleanup also names
  the raw resumption setter clearly and removes the obsolete test harness cast.
