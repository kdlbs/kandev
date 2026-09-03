// This file starts with `mobile-` so Playwright runs it on the Pixel 5 project.
import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  FIRST_PROMPT_MARKER,
  SECOND_PROMPT_MARKER,
  seedLongPromptHistory,
} from "../../helpers/prompt-history-long-seed";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/** CDP touch-scrolls the element upward, which scrolls its content DOWN
 * (revealing older content), matching the repo's mobile scroll convention. */
async function touchScrollDown(page: Page, scrollElement: Locator) {
  const box = await scrollElement.boundingBox();
  if (!box) throw new Error("scroll container has no bounding box");
  const cdp = await page.context().newCDPSession(page);
  const centerX = box.x + box.width / 2;
  const startY = box.y + box.height - 20;
  const endY = box.y + 20;
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: centerX, y: startY }],
  });
  for (let i = 1; i <= 8; i++) {
    const y = startY + ((endY - startY) * i) / 8;
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: centerX, y }],
    });
  }
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchEnd",
    touchPoints: [],
  });
}

test.describe("Prompt history panel on mobile", () => {
  test("opens from Panels and returns to Chat for a prompt jump", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const seedPrompt = "Mobile prompt history seeded prompt";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile prompt history task",
      seedData.agentProfileId,
      {
        description: seedPrompt,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("Mobile prompt history task did not create a session");

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 45_000 },
      )
      .toBe(true);

    const { messages } = await apiClient.listSessionMessages(task.session_id);
    const promptMessage = messages.find(
      (message) => message.author_type === "user" && message.content.includes(seedPrompt),
    );
    if (!promptMessage) throw new Error("Mobile prompt history prompt was not persisted");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const panelsButton = testPage.getByRole("button", { name: "Panels" });
    await expect(panelsButton).toBeVisible({ timeout: 15_000 });
    expect((await panelsButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await panelsButton.tap();

    const historyOption = testPage.getByTestId("mobile-prompt-history-option");
    await expect(historyOption).toBeVisible({ timeout: 10_000 });
    expect((await historyOption.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await historyOption.tap();

    const historyPanel = testPage.getByTestId("prompt-history-panel");
    await expect(historyPanel).toBeVisible({ timeout: 10_000 });
    const row = testPage.getByTestId("prompt-history-row-0");
    await expect(row).toContainText(seedPrompt);
    // The single seeded prompt is the session's very first: #1.
    await expect(row.getByTestId("prompt-history-number-0")).toHaveText("#1");
    const prompt = row.locator('[role="button"]').first();
    const promptBox = await prompt.boundingBox();
    expect(promptBox?.height).toBeGreaterThanOrEqual(44);

    await prompt.tap();
    await expect(testPage.locator(`#msg-${promptMessage.id}`)).toBeAttached();
  });

  test("auto-loads a long history via touch scroll with no button", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const taskId = await seedLongPromptHistory(apiClient, {
      workspaceId: seedData.workspaceId,
      agentProfileId: seedData.agentProfileId,
      workflowId: seedData.workflowId,
      startStepId: seedData.startStepId,
      repositoryId: seedData.repositoryId,
    });

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const panelsButton = testPage.getByRole("button", { name: "Panels" });
    await expect(panelsButton).toBeVisible({ timeout: 15_000 });
    await panelsButton.tap();
    const historyOption = testPage.getByTestId("mobile-prompt-history-option");
    await expect(historyOption).toBeVisible({ timeout: 10_000 });
    await historyOption.tap();

    const panel = testPage.getByTestId("prompt-history-panel");
    await expect(panel).toBeVisible({ timeout: 10_000 });
    await expect(panel.getByTestId("prompt-history-number-0")).toHaveText("#121");
    await expect(panel.getByText(SECOND_PROMPT_MARKER)).toHaveCount(0);
    // Touch gestures must target the panel's inner scroll container (the
    // outer root is only the positioned overlay wrapper).
    const scroller = panel.getByTestId("prompt-history-scroll");
    await expect(scroller).toBeVisible();

    // Arm a route handler that holds older-page requests, then touch-scroll:
    // the panel-triggered request is held, the loading row shows, and the
    // marker is still absent.
    let releaseHeld: (() => void) | null = null;
    const olderRequestGate = new Promise<void>((resolve) => {
      releaseHeld = resolve;
    });
    let heldOlderRequests = 0;
    await testPage.route("**/api/v1/task-sessions/*/messages*", async (route) => {
      if (route.request().url().includes("before=")) {
        heldOlderRequests += 1;
        await olderRequestGate;
      }
      await route.continue();
    });

    // The panel holds ~100 rows, so one touch gesture cannot reach the bottom
    // sentinel: repeat touch-scrolling until the panel-triggered older-page
    // request fires (and is held), then assert the loading state.
    let scrolls = 0;
    while (
      scrolls < 10 &&
      (await panel.getByTestId("prompt-history-loading-older").count()) === 0
    ) {
      await touchScrollDown(testPage, scroller);
      scrolls += 1;
    }
    await expect(panel.getByTestId("prompt-history-loading-older")).toBeVisible({
      timeout: 10_000,
    });
    await expect(panel.getByText(SECOND_PROMPT_MARKER)).toHaveCount(0);
    expect(heldOlderRequests).toBeGreaterThanOrEqual(1);
    releaseHeld?.();

    // Repeat touch scrolling until the #1 description row renders.
    const firstRow = panel.locator('[data-testid^="prompt-history-row-"]', {
      hasText: FIRST_PROMPT_MARKER,
    });
    for (let attempt = 0; attempt < 10 && (await firstRow.count()) === 0; attempt++) {
      const rowsBefore = await panel.locator('[data-testid^="prompt-history-row-"]').count();
      await touchScrollDown(testPage, scroller);
      await expect
        .poll(async () => await panel.locator('[data-testid^="prompt-history-row-"]').count(), {
          timeout: 5_000,
        })
        .toBeGreaterThan(rowsBefore);
    }
    await expect(firstRow).toBeAttached({ timeout: 10_000 });
    await expect(firstRow.locator('[data-testid^="prompt-history-number-"]')).toHaveText("#1");

    // The first prompt is outside the initial transcript page. Selecting it
    // must load its around-window before Chat attempts to scroll.
    const firstPromptMessageId = await firstRow
      .locator("[data-message-id]")
      .getAttribute("data-message-id");
    if (!firstPromptMessageId) throw new Error("First prompt row has no message id");
    await firstRow.locator('[role="button"]').first().tap();
    await expect(testPage.locator(`#msg-${firstPromptMessageId}`)).toBeAttached({
      timeout: 10_000,
    });

    // No load-more button inside the panel.
    await expect(panel.getByTestId("load-older-messages")).toHaveCount(0);
  });
});
