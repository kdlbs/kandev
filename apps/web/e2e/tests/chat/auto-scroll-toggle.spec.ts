import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

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
  const script = Array.from(
    { length: messageCount },
    (_, i) =>
      `e2e:message("Filler message ${i + 1} - lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua")`,
  ).join("\n");

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

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

function chatList(testPage: Page) {
  return testPage.locator(".chat-message-list:visible").first();
}

async function waitForOverflow(testPage: Page) {
  await expect
    .poll(async () => chatList(testPage).evaluate((el) => el.scrollHeight - el.clientHeight), {
      timeout: 15_000,
      message: "Waiting for chat to overflow",
    })
    .toBeGreaterThan(200);
}

test.describe("Transcript auto-scroll toggle", () => {
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

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
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

    // Re-enabling catches the view up to the bottom because new content
    // (the earlier filler messages already below view) is still there.
    await toggleAfter.click();
    await expect(toggleAfter).toHaveAttribute("aria-pressed", "true");
    await expect
      .poll(
        async () => listAfter.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight),
        { timeout: 5_000, message: "re-enabling should catch the view up to the bottom" },
      )
      .toBeLessThan(10);
  });
});
