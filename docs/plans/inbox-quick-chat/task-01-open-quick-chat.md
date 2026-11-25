---
id: "01-open-quick-chat"
title: "Open the matching Quick Chat from Inbox"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-INBOX-OPEN-TASK-001
acceptance_criteria:
  - AC-UI-INBOX-OPEN-TASK-001.1
  - AC-UI-INBOX-OPEN-TASK-001.2
system_design:
  - ../../specs/ui/system-design/inbox-open-task.md
---

# Open the matching Quick Chat from Inbox

## Scope

Add a shared Inbox open action that uses the current Quick Chat inventory and
existing dialog opener. Connect every **Open task** control in Needs you,
Failed, and History. Preserve native link behavior for modified clicks. Use
the persistent menu trigger for focus return when the menu item unmounts.

Update the Inbox paragraph in `docs/public/sessions-and-review.md` after
verification. Exclude changes to Office Inbox, task and session lifecycle,
API payloads, and dialog styling.

## Acceptance

1. Opening a matching Quick Chat row in each Inbox tab selects the correct
   conversation. The Inbox URL stays in place.
2. An ordinary task, or a row with no matching conversation in the workspace,
   uses the existing task URL and preserves its session query when present.
3. Desktop and touch activation open the existing dialog. Closing it returns
   to the Inbox tab and restores focus to a persistent control.

## Files and dependencies

- `apps/web/components/needs-you-inbox/needs-you-inbox-row.tsx` and
  `failed-inbox-row.tsx` in the same directory.
- `apps/web/components/inbox-history/inbox-history-row.tsx`.
- Shared Inbox link and its component test in
  `apps/web/components/needs-you-inbox/inbox-open-task-link.tsx` and
  `inbox-open-task-link.test.tsx`.
- Existing desktop and phone Needs you Inbox browser specs in
  `apps/web/e2e/tests/chat/`.

Use `AppLink`, `linkToTask`, the Quick Chat store action, and the dialog's
focus helper. The store must already contain the conversation. Match IDs at
activation so a stale row cannot recreate a deleted chat.

## ASCII UI preview

UI-01 from the [plan](plan.md#ascii-ui-preview).

```text
Desktop                          Phone
Inbox > Open task                Inbox > Actions > Open task
  Quick Chat floating dialog       Quick Chat full-height dialog
  Tabs, transcript, composer      Tabs, transcript, composer
  Close > same Inbox tab           Close > same Inbox tab
```

The existing dialog owns scrolling, composer placement, and the close control.
Check both layouts in the browser.

## Verification

First add a regression named `opens the target Quick Chat from Inbox` to each
selected Playwright spec. Use the existing Quick Chat setup helpers and send
`/e2e:clarification-multi`. Create a second Quick Chat to check exact
selection. Verify that the desktop action and phone menu action open the
conversation with the pending question. Verify that closing returns to Inbox.
Run the desktop regression before the production edit. It should find the
task route and fail the Quick Chat assertion.

Cover task-only and task-plus-session matches in the shared link test. Cover
wrong session, wrong workspace, missing match, and modified clicks. Keep
the existing row tests for regular task URLs and mobile control reachability.
Use a Failed row fixture with a matching tab in the component tests because
the Failed bucket requires task state FAILED.

From `apps/`:

```sh
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test inbox-open-task-link needs-you-inbox-row failed-inbox-row inbox-history-row needs-you-inbox-page-client failed-inbox-tab-panel inbox-history-list
```

From `apps/web/`, run the projects in order:

```sh
pnpm e2e:run --project chromium tests/chat/needs-you-inbox-answer.spec.ts tests/chat/inbox-failed-tab.spec.ts
pnpm e2e:run --project mobile-chrome tests/chat/mobile-needs-you-inbox-answer.spec.ts tests/chat/mobile-inbox-failed-tab.spec.ts
```

## Results

The Failed row component regression failed before the patch. Activating the
row changed the URL to `/t/t1` despite a matching Quick Chat conversation.
The initial desktop browser run stopped during build, so the component test
provided the pre-patch regression evidence.

The managed desktop run passed 6 tests. The managed phone run passed 3 tests.
Both runs used systemd with `MemoryMax=6000000000`. The complete Send Now
fixture build exceeded the cap. A focused build compiled the current
orchestrator sources and the Send Now fixture test. The browser runs used
`--host --no-build` after building the SPA and packaging the fixture plugin.
They used a loopback backend with the task session's Kandev environment removed.

The capped Vitest run passed 94 tests. Web type checking, changed-file
formatting and lint, and specification validation passed.
