import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionDone } from "../../helpers/session";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { dwell } from "../../helpers/causal-waits";
import { waitForStableActiveSession } from "../../helpers/session-store";

type E2EMessageStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      messages: { bySession: Record<string, Array<{ content: string }>> };
    };
  };
};

const INACTIVE_SESSION_MARKER = "INACTIVE-AUTO-SCROLL-CATCH-UP-7M2P";

function overflowScript(prefix: string, messageCount = 30): string {
  return Array.from(
    { length: messageCount },
    (_, i) =>
      `e2e:message("${prefix} ${i + 1} - lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua")`,
  ).join("\n");
}

/**
 * Seed a task whose mock-agent script emits enough distinct messages to
 * overflow the chat list, then open it and wait for the turn to finish.
 */
async function seedOverflowingTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  messageCount = 30,
): Promise<SessionPage> {
  const script = overflowScript("Filler message", messageCount);

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: script,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  // This fixture needs persisted history, not a live initial stream. Under CI
  // load, rendering every seed frame can make the turn outlive the UI's idle
  // wait and fail setup while the agent is still legitimately running.
  await waitForSessionDone(
    apiClient,
    task.id,
    task.session_id,
    "overflow seed session should finish before opening the transcript",
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

/** Seeds two real sessions with overflowing persisted histories. */
async function seedTwoSessionOverflowingTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{
  session: SessionPage;
  taskId: string;
  firstSessionId: string;
  secondSessionId: string;
}> {
  const firstScript = overflowScript("First session history");
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Inactive session transcript auto-scroll",
    seedData.agentProfileId,
    {
      description: firstScript,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await waitForSessionDone(
    apiClient,
    task.id,
    task.session_id,
    "first overflowing session should finish before opening the task",
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.openNewSessionDialog();
  await session.newSessionPromptInput().fill(overflowScript("Second session history"));
  await session.newSessionStartButton().click();
  await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

  let secondSessionId: string | null = null;
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const second = sessions.find((candidate) => candidate.id !== task.session_id);
        if (!second) return false;
        secondSessionId = second.id;
        return ["COMPLETED", "WAITING_FOR_INPUT"].includes(second.state);
      },
      { timeout: 120_000, message: "second overflowing session should finish" },
    )
    .toBe(true);
  if (!secondSessionId) throw new Error("second session was not created");

  return {
    session,
    taskId: task.id,
    firstSessionId: task.session_id,
    secondSessionId,
  };
}

function chatList(testPage: Page) {
  return testPage.locator(".chat-message-list:visible").first();
}

const AUTO_SCROLL_END_TOLERANCE_PX = 50;

async function waitForOverflow(testPage: Page) {
  await expect
    .poll(async () => chatList(testPage).evaluate((el) => el.scrollHeight - el.clientHeight), {
      timeout: 15_000,
      message: "Waiting for chat to overflow",
    })
    .toBeGreaterThan(200);
}

test.describe("Transcript auto-scroll toggle", () => {
  test("inactive session transcript reopens at the bottom after refresh and hidden updates", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);

    const { session, firstSessionId, secondSessionId } = await seedTwoSessionOverflowingTask(
      testPage,
      apiClient,
      seedData,
    );
    await expect(session.sessionTabBySessionId(firstSessionId)).toBeVisible({ timeout: 15_000 });
    await expect(session.sessionTabBySessionId(secondSessionId)).toBeVisible({ timeout: 15_000 });

    // Leave the first transcript inactive before a refresh. This reproduces
    // Dockview's persistent portal with populated content and zero geometry.
    await session.sessionTabBySessionId(secondSessionId).click();
    await waitForStableActiveSession(testPage, secondSessionId);
    await testPage.reload({ waitUntil: "domcontentloaded" });

    const refreshedSession = new SessionPage(testPage);
    await refreshedSession.waitForLoad();
    await expect(refreshedSession.sessionTabBySessionId(firstSessionId)).toBeVisible({
      timeout: 15_000,
    });
    await expect(refreshedSession.sessionTabBySessionId(secondSessionId)).toBeVisible({
      timeout: 15_000,
    });
    await refreshedSession.sessionTabBySessionId(secondSessionId).click();
    await waitForStableActiveSession(testPage, secondSessionId);

    // Activating the previously inactive, enabled transcript must place it at
    // the newest message after SessionPanelContent restores its old offset.
    await refreshedSession.sessionTabBySessionId(firstSessionId).click();
    await waitForStableActiveSession(testPage, firstSessionId);
    const firstChat = testPage.locator(
      `[data-testid='session-chat'][data-session-id=${JSON.stringify(firstSessionId)}]`,
    );
    const firstList = firstChat.locator(".chat-message-list");
    await expect
      .poll(async () => firstList.evaluate((el) => el.scrollHeight - el.clientHeight), {
        timeout: 15_000,
        message: "first inactive transcript should overflow after activation",
      })
      .toBeGreaterThan(200);
    await expect
      .poll(
        async () => firstList.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight),
        {
          timeout: 15_000,
          message: "activating an inactive enabled transcript should restore the bottom",
        },
      )
      .toBeLessThan(10);

    // Deliver new content while that bottom-following transcript is hidden.
    // Wait for the store event before switching back so the assertion proves
    // activation reconciliation, not a delayed initial fetch.
    await refreshedSession.sessionTabBySessionId(secondSessionId).click();
    await waitForStableActiveSession(testPage, secondSessionId);
    await apiClient.seedSessionMessage(firstSessionId, {
      type: "message",
      content: INACTIVE_SESSION_MARKER,
      authorType: "agent",
    });
    await expect
      .poll(
        async () =>
          testPage.evaluate(
            ({ sid, marker }) => {
              const store = (window as E2EMessageStoreWindow).__KANDEV_E2E_STORE__;
              return store
                ?.getState()
                .messages.bySession[sid]?.some((message) => message.content === marker);
            },
            { sid: firstSessionId, marker: INACTIVE_SESSION_MARKER },
          ),
        {
          timeout: 15_000,
          message: "hidden message should arrive in the inactive transcript cache",
        },
      )
      .toBe(true);

    await refreshedSession.sessionTabBySessionId(firstSessionId).click();
    await waitForStableActiveSession(testPage, firstSessionId);
    await expect(
      refreshedSession.activeChat().getByText(INACTIVE_SESSION_MARKER, { exact: true }),
    ).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(
        async () => firstList.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight),
        {
          timeout: 15_000,
          message: "hidden content should catch up an enabled transcript on activation",
        },
      )
      .toBeLessThan(10);

    // A reader-owned position must survive the same tab round trip. Dispatch
    // a native scroll event so both the transcript coordinator and the
    // generic panel restorer capture the exact reader position.
    const targetScrollTop = await firstList.evaluate((el) => {
      const target = Math.floor((el.scrollHeight - el.clientHeight) / 2);
      el.scrollTop = target;
      el.dispatchEvent(new Event("scroll"));
      return el.scrollTop;
    });
    expect(targetScrollTop).toBeGreaterThan(100);
    const toggle = firstChat.getByTestId("auto-scroll-toggle-button");
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    await refreshedSession.sessionTabBySessionId(secondSessionId).click();
    await waitForStableActiveSession(testPage, secondSessionId);
    await refreshedSession.sessionTabBySessionId(firstSessionId).click();
    await waitForStableActiveSession(testPage, firstSessionId);
    await expect
      .poll(
        async () =>
          firstList.evaluate((el, expected) => Math.abs(el.scrollTop - expected), targetScrollTop),
        {
          timeout: 15_000,
          message: "disabled reader position should survive session activation",
        },
      )
      .toBeLessThanOrEqual(20);
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
  });

  test("can be hidden without changing the default auto-scroll state", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ show_transcript_auto_scroll_control: false });
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Auto-scroll Toggle Hidden",
    );
    await waitForOverflow(testPage);

    await expect(session.chatStatusBar().getByTestId("auto-scroll-toggle-button")).toHaveCount(0);
    const list = session.activeChat().locator(".chat-message-list");
    await expect
      .poll(async () => {
        return list.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight);
      })
      .toBeLessThan(AUTO_SCROLL_END_TOLERANCE_PX);
  });

  test("is visible next to Share, enabled by default, and toggles on click", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Auto-scroll Toggle Basic",
      2,
    );

    const statusBar = session.chatStatusBar();
    const toggle = statusBar.getByTestId("auto-scroll-toggle-button");
    await expect(toggle).toBeVisible({ timeout: 10_000 });
    await expect(toggle).toHaveAttribute("aria-pressed", "true");

    const icon = toggle.getByTestId("auto-scroll-toggle-icon");
    await expect(icon).toHaveClass(/text-green-600/);

    // Sits immediately to the left of Share within the same right-aligned cluster.
    const shareButton = statusBar.getByTestId("share-task-button");
    if (await shareButton.isVisible()) {
      const toggleBox = await toggle.boundingBox();
      const shareBox = await shareButton.boundingBox();
      expect(toggleBox).not.toBeNull();
      expect(shareBox).not.toBeNull();
      if (toggleBox && shareBox) {
        expect(toggleBox.x).toBeLessThan(shareBox.x);
      }
    }

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    await expect(icon).not.toHaveClass(/text-green-600/);

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await expect(icon).toHaveClass(/text-green-600/);
  });

  test("disabling freezes the position and suppresses auto-scroll for new messages", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Auto-scroll Toggle Freeze",
    );
    await waitForOverflow(testPage);

    const list = chatList(testPage);
    const targetScrollTop = await list.evaluate((el) => {
      el.scrollTop = Math.floor((el.scrollHeight - el.clientHeight) / 2);
      return el.scrollTop;
    });
    expect(targetScrollTop).toBeGreaterThan(100);

    const toggle = session.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    // A brand-new message arrives (a real follow-up turn over the live WS
    // pipeline) while scrolled away from the bottom and disabled.
    await session.sendMessage('e2e:message("New content while disabled")');
    await expect(session.chat.getByText("New content while disabled").last()).toBeVisible({
      timeout: 15_000,
    });

    // The view must not have jumped — scrollTop stays put even though the
    // transcript grew taller.
    await expect
      .poll(async () => list.evaluate((el) => el.scrollTop), { timeout: 2_000 })
      .toBeLessThan(targetScrollTop + 10);
    expect(await list.evaluate((el) => el.scrollTop)).toBeGreaterThan(targetScrollTop - 10);
  });

  test("enabled auto-scroll stays at the bottom for live messages", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Auto-scroll Toggle Enabled Live Message",
    );
    await waitForOverflow(testPage);

    const list = chatList(testPage);
    const toggle = session.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await expect(toggle).toHaveAttribute("aria-pressed", "true");

    await session.sendMessage('e2e:message("New content while enabled")');
    await expect(session.chat.getByText("New content while enabled").last()).toBeVisible({
      timeout: 15_000,
    });
    await expect
      .poll(async () => list.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight), {
        timeout: 5_000,
        message: "enabled auto-scroll should stay at the bottom after live content",
      })
      .toBeLessThan(10);
  });

  test("disabling while genuinely at the bottom still freezes the view when new content arrives", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Auto-scroll Toggle Bottom Anchor",
    );
    await waitForOverflow(testPage);

    const list = chatList(testPage);
    // Establish the scenario's true-bottom precondition explicitly. The
    // sticky prompt bar can settle after the renderer's initial auto-scroll,
    // which is unrelated to the disabled-state behavior under test.
    await expect
      .poll(
        async () =>
          list.evaluate((el) => {
            el.scrollTop = el.scrollHeight;
            return el.scrollHeight - el.scrollTop - el.clientHeight;
          }),
        {
          timeout: 5_000,
          message: "expected to be at the bottom before disabling",
        },
      )
      .toBeLessThan(5);
    const bottomScrollTop = await list.evaluate((el) => el.scrollTop);

    const toggle = session.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    await expect(list).toHaveCSS("overflow-anchor", "none");

    // A new message arrives while disabled from the true bottom. The
    // browser's native bottom overflow-anchor must not override the
    // toggle: without disabling it too, the anchor keeps itself pinned to
    // the viewport, dragging scrollTop down to chase the new content.
    await session.sendMessage('e2e:message("New content while disabled at bottom")');
    await expect(session.chat.getByText("New content while disabled at bottom").last()).toBeVisible(
      { timeout: 15_000 },
    );

    await expect
      .poll(
        async () =>
          list.evaluate(
            (el, expectedScrollTop) => Math.abs(el.scrollTop - expectedScrollTop),
            bottomScrollTop,
          ),
        { timeout: 2_000 },
      )
      .toBeLessThanOrEqual(2);
  });

  test("preserves the frozen scroll position across navigating away and back", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Auto-scroll Toggle Nav",
    );
    await waitForOverflow(testPage);

    const list = chatList(testPage);
    const targetScrollTop = await list.evaluate((el) => {
      el.scrollTop = Math.floor((el.scrollHeight - el.clientHeight) / 2);
      return el.scrollTop;
    });
    expect(targetScrollTop).toBeGreaterThan(100);

    const toggle = session.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    // Navigate away to the kanban board, then back into the same task —
    // this remounts the chat panel via dockview's layout rebuild.
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardByTitle("Auto-scroll Toggle Nav");
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const sessionAfter = new SessionPage(testPage);
    await sessionAfter.waitForLoad();

    const listAfter = chatList(testPage);
    await listAfter.waitFor({ state: "visible", timeout: 10_000 });

    // Position is restored, not reset to the bottom — and the toggle itself
    // still reflects the disabled preference.
    await expect
      .poll(async () => listAfter.evaluate((el) => el.scrollTop), {
        timeout: 5_000,
        message: "scroll position should be restored after navigating back",
      })
      .toBeGreaterThan(targetScrollTop - 20);
    const toggleAfter = sessionAfter.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await expect(toggleAfter).toHaveAttribute("aria-pressed", "false");

    // Re-enabling must NOT jump to the bottom: nothing new arrived while
    // disabled — the unseen content below is pre-existing history the user
    // was already scrolled away from before disabling, not progression.
    await toggleAfter.click();
    await expect(toggleAfter).toHaveAttribute("aria-pressed", "true");
    await dwell(
      testPage,
      300,
      "negative-assertion",
      'asserts a negative (no jump to bottom); there is no event for "the scroll that must not happen", so a regression needs the smooth-scroll window to elapse before the position is sampled',
    );
    expect(await listAfter.evaluate((el) => el.scrollTop)).toBeGreaterThan(targetScrollTop - 20);
  });

  test("re-enabling does not jump to the bottom when nothing progressed while disabled", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Auto-scroll Toggle No Progress",
    );
    await waitForOverflow(testPage);

    const list = chatList(testPage);
    const targetScrollTop = await list.evaluate((el) => {
      el.scrollTop = Math.floor((el.scrollHeight - el.clientHeight) / 2);
      return el.scrollTop;
    });
    expect(targetScrollTop).toBeGreaterThan(100);

    const toggle = session.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    // No new message arrives — the user is just reading older history that
    // already existed below their view before they disabled.
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await dwell(
      testPage,
      300,
      "negative-assertion",
      'asserts a negative (no jump to bottom); there is no event for "the scroll that must not happen", so a regression needs the smooth-scroll window to elapse before the position is sampled',
    );
    expect(await list.evaluate((el) => el.scrollTop)).toBeGreaterThan(targetScrollTop - 20);
    expect(await list.evaluate((el) => el.scrollTop)).toBeLessThan(targetScrollTop + 20);
  });

  test("re-enabling does not jump when the user scrolls further away themselves while disabled", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Auto-scroll Toggle Manual Scroll",
    );
    await waitForOverflow(testPage);

    const list = chatList(testPage);
    await list.evaluate((el) => {
      el.scrollTop = Math.floor((el.scrollHeight - el.clientHeight) / 2);
    });

    const toggle = session.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    // The user scrolls further up on their own — no new content arrives.
    const scrolledUpTarget = await list.evaluate((el) => {
      el.scrollTop = 0;
      return el.scrollTop;
    });
    expect(scrolledUpTarget).toBe(0);

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await dwell(
      testPage,
      300,
      "negative-assertion",
      'asserts a negative (no jump to bottom); there is no event for "the scroll that must not happen", so a regression needs the smooth-scroll window to elapse before the position is sampled',
    );
    expect(await list.evaluate((el) => el.scrollTop)).toBe(0);
  });

  test("catches up on re-enable when content arrives after remounting while already disabled", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Auto-scroll Toggle Remount Progress",
    );
    await waitForOverflow(testPage);

    const list = chatList(testPage);
    const targetScrollTop = await list.evaluate((el) => {
      el.scrollTop = Math.floor((el.scrollHeight - el.clientHeight) / 2);
      return el.scrollTop;
    });
    expect(targetScrollTop).toBeGreaterThan(100);

    const toggle = session.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    // Navigate away and back — the panel remounts while the persisted
    // preference is still disabled, exercising the mount-time baseline
    // initialization (not a live enabled->disabled transition).
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardByTitle("Auto-scroll Toggle Remount Progress");
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const sessionAfter = new SessionPage(testPage);
    await sessionAfter.waitForLoad();
    const toggleAfter = sessionAfter.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await expect(toggleAfter).toHaveAttribute("aria-pressed", "false");

    const listAfterRemount = chatList(testPage);
    await listAfterRemount.waitFor({ state: "visible", timeout: 10_000 });
    await expect
      .poll(async () => listAfterRemount.evaluate((el) => el.scrollTop), {
        timeout: 5_000,
        message: "scroll position should be restored after remounting while disabled",
      })
      .toBeGreaterThan(targetScrollTop - 20);

    // Genuinely new content arrives now, after the remount, while still disabled.
    await sessionAfter.sendMessage('e2e:message("New content after remount while disabled")');
    await expect(
      sessionAfter.chat.getByText("New content after remount while disabled").last(),
    ).toBeVisible({ timeout: 15_000 });

    const listAfter = chatList(testPage);
    await toggleAfter.click();
    await expect(toggleAfter).toHaveAttribute("aria-pressed", "true");
    await expect
      .poll(
        async () => listAfter.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight),
        { timeout: 5_000, message: "re-enabling should catch up to the genuinely new content" },
      )
      .toBeLessThan(10);
  });
});
