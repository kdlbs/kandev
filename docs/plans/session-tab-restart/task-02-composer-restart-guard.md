---
id: "02-composer-restart-guard"
title: "Guard composer submission during restart"
status: done
wave: 2
depends_on:
  - "01-session-restart"
plan: "plan.md"
requirements:
  - REQ-TASKS-SESSION-TAB-RESTART-001
acceptance_criteria:
  - AC-TASKS-SESSION-TAB-RESTART-001.6
system_design:
  - ../../specs/tasks/system-design/session-tab-restart.md
---

# Task 02: Guard composer submission during restart

The user approved this follow-up after testing PR #4520. The existing pending
set protects lifecycle menus, but the composer and its submit handler do not
read it. A user can submit while Stop and Resume are in progress.

## Scope and ownership

Update the restart hook, `chat-input-area`, `composer-status-notices`, `use-composer-props`,
`chat-input-container`, and `use-chat-input-container`. Extend their focused
tests and both existing session restart E2E files. Reuse the localized progress
copy and existing composer geometry. Update public session guidance.

Share the pending model with `lib/ws/handlers/tasks.ts` and `agent-session.ts`
through `lib/session/session-restart-state.ts`. Keep the selected target and pin
through automatic handoff events without changing backend primary promotion.
Cover both event paths and a pending different session in their handler tests.

No backend, persistence, recovery-policy, or cross-browser coordination changes.
Work sequentially in the primary session.

## Acceptance

1. Acquire the guard before Stop. The target cannot submit through Send,
   keyboard shortcuts, or an earlier shared submit callback while either
   request is pending. Other sessions can still submit.
2. Preserve editable draft text and attachments. Show the pending status inline.
   Keep the editor available through the temporary CANCELLED projection.
3. Release the transient guard after success or failure. Existing startup queue
   readiness and recovery rules retain control after release. Never auto-submit.
4. Check desktop and native phone paths against held WebSocket responses.

## UI preview

Both surfaces retain the shipped composer and its current controls:

```text
Restarting session...           <- visible status, also announced
[Keep editing this draft]       <- editor stays editable
[attachments retained] [Send]   <- Send disabled during restart
```

The phone uses the existing bottom composer and Sessions row menu. The notice
adds no new hover dependency, dialog, motion, or custom control geometry.

## Verification

Write and run the failing hook regressions before production edits. Check the
real pending set through a deferred restart, not a mocked restart predicate.
Browser tests hold Stop and Resume responses separately and inspect actual
message/queue traffic while submission is blocked.

Run from the repository root:

```bash
pnpm --dir apps/web exec vitest run hooks/domains/session/use-session-actions.test.ts components/task/chat/chat-input-area.test.tsx components/task/chat/chat-input-area-restart.test.tsx components/task/chat/use-chat-input-container.test.ts components/task/chat/use-composer-props.test.tsx components/task/chat/chat-input-container.test.tsx components/task/chat/chat-input-body.test.tsx components/task/chat/use-chat-input-state.test.ts lib/ws/handlers --maxWorkers=2
NODE_OPTIONS=--max-old-space-size=6144 pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run i18n:ratchet
pnpm --dir apps/web e2e:run --host --project chromium tests/session/session-tab-restart.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/session/mobile-session-tab-restart.spec.ts -- --retries=0
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the established clean launch environment and Chromium library paths on this
host. Run desktop and phone E2E sequentially. Also run ESLint and Prettier on
changed source and tests. After the final commit, publish current desktop and
phone screenshots, push the verified head, and collect fresh CI/review evidence.

## Results

Implementation and local verification completed on 2026-10-19 in the primary
session. No subagents were used.

- The initial hook regressions failed because submission remained enabled and
  an earlier callback accepted the prompt. The previous browser build also
  lacked the pending composer notice.
- A stronger two-session browser regression exposed an automatic switch to the
  live sibling during Stop. Targeted regressions failed in both WebSocket
  selection paths. The shared pending model now retains the selected target
  and its pin while preserving state projection and normal primary promotion.
- The expanded Vitest command above passed: 782 tests across 75 files. Coverage
  includes both event paths, a different pending session as a positive control,
  the real pending set, an earlier submit callback, both pending phases, an
  unaffected sibling, and existing attachment preservation on a false result.
- Changed-file ESLint passed with zero warnings. Prettier and the 6144 MiB
  typecheck passed. Localization checks and the new-code ratchet passed.
- A fresh managed desktop build passed all 5 restart tests with retries disabled.
  The phone run reused those assets and passed both tests with retries disabled.
  Both new guard tests use two live sessions and hold Stop and Resume responses
  separately. The draft stays editable, no prompt or queue request is sent,
  and each test sends one message after release as a positive control. Existing
  history, explicit navigation, and recovery scenarios remain covered.
- Public-doc validation passed for 48 pages. Specification catalog validation,
  full specification lint, and whitespace checks passed.

The existing composer status row is the visual exemplar on both surfaces. No
new motion or touch-control geometry was added. Captures wait for finite UI
animations before recording the pending notice and menu.

Public docs updated: `docs/public/sessions-and-review.md` (how-to guidance).
Final commit checks, current screenshot publication, test-instance refresh, and
remote CI/review remain delivery steps. Exact-head remote evidence belongs in
the PR and handoff.
