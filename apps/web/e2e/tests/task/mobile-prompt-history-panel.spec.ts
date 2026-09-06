// This file starts with `mobile-` so Playwright runs it on the Pixel 5 project.
import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  FIRST_PROMPT_MARKER,
  MIDDLE_PROMPT_MARKER,
  SECOND_PROMPT_MARKER,
  seedLongPromptHistory,
} from "../../helpers/prompt-history-long-seed";
async function installTargetScrollCounter(
  page: Page,
  messageId: string,
  displaceAfterFirstScroll = false,
) {
  await page.evaluate(
    ({ id, displace }) => {
      const windowWithCalls = window as typeof window & {
        __promptHistoryTargetScrollCalls?: number;
        __promptHistoryScrollOptions?: ScrollIntoViewOptions[];
        __promptHistoryScrollTimes?: number[];
        __promptHistoryDisplacementApplied?: boolean;
        __promptHistoryDisplacementOffset?: number;
      };
      const original = Element.prototype.scrollIntoView;
      const targetSelector = `[id="msg-${CSS.escape(id)}"]`;
      const applyDisplacement = () => {
        const current = document.querySelector<HTMLElement>(targetSelector);
        const list = current?.closest<HTMLElement>(".chat-message-list");
        if (!current || !list) return;
        current.style.setProperty("transform", "translateY(200px)", "important");
        windowWithCalls.__promptHistoryDisplacementOffset =
          current.getBoundingClientRect().top - list.getBoundingClientRect().top;
        windowWithCalls.__promptHistoryDisplacementApplied = true;
      };
      windowWithCalls.__promptHistoryTargetScrollCalls = 0;
      windowWithCalls.__promptHistoryScrollOptions = [];
      windowWithCalls.__promptHistoryScrollTimes = [];
      windowWithCalls.__promptHistoryDisplacementApplied = false;
      Element.prototype.scrollIntoView = function (...args) {
        const isTarget = this.id === `msg-${id}`;
        let list: Element | null = null;
        let calls = 0;
        if (isTarget) {
          calls = (windowWithCalls.__promptHistoryTargetScrollCalls ?? 0) + 1;
          windowWithCalls.__promptHistoryTargetScrollCalls = calls;
          windowWithCalls.__promptHistoryScrollOptions?.push(args[0]);
          windowWithCalls.__promptHistoryScrollTimes?.push(performance.now());
          list = this.closest(".chat-message-list");
        }
        if (
          displace &&
          isTarget &&
          calls === 2 &&
          windowWithCalls.__promptHistoryDisplacementApplied
        ) {
          document.querySelectorAll<HTMLElement>(targetSelector).forEach((element) => {
            element.style.removeProperty("transform");
          });
        }
        const result = original.apply(this, args);
        if (displace && isTarget && calls === 1 && list instanceof HTMLElement) {
          window.setTimeout(() => {
            if (list.isConnected) applyDisplacement();
          }, 50);
        }
        return result;
      };
    },
    { id: messageId, displace: displaceAfterFirstScroll },
  );
}
const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const MOBILE_ALIAS = "mobile-history-alias";

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
async function revealPromptHistoryTarget(page: Page, panel: Locator, targetBubble: Locator) {
  const scroller = panel.getByTestId("prompt-history-scroll");
  await expect(scroller).toBeVisible();
  for (let attempt = 0; attempt < 10; attempt++) {
    if ((await targetBubble.count()) > 0) return;
    await touchScrollDown(page, scroller);
    await targetBubble
      .first()
      .waitFor({ state: "attached", timeout: 5_000 })
      .catch(() => undefined);
  }
  if ((await targetBubble.count()) > 0) return;
  throw new Error("prompt history target did not load after 10 touch-scroll attempts");
}

test.describe("Prompt history panel on mobile", () => {
  test.afterEach(async ({ apiClient }) => {
    const { prompts } = await apiClient.listPrompts();
    for (const prompt of prompts) {
      if (!prompt.builtin && prompt.name === MOBILE_ALIAS) {
        await apiClient.deletePrompt(prompt.id).catch(() => undefined);
      }
    }
  });
  test("opens from Panels and returns to Chat for a prompt jump", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await apiClient.createPrompt(MOBILE_ALIAS, "Mobile history alias content");
    const seedPrompt = `@${MOBILE_ALIAS}`;
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
    const chip = row.getByTestId("custom-prompt-mention");
    await expect(chip).toBeVisible({ timeout: 15_000 });
    await expect(chip).toHaveAttribute("data-prompt-name", MOBILE_ALIAS);
    await chip.tap();
    await expect(testPage.getByText("Mobile history alias content")).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(testPage.getByText("Mobile history alias content")).toHaveCount(0);
    // The single seeded prompt is the session's very first: #1.
    await expect(row.getByTestId("prompt-history-number-0")).toHaveText("#1");
    const prompt = row.locator("[data-message-id]");
    const promptBox = await prompt.boundingBox();
    expect(promptBox?.height).toBeGreaterThanOrEqual(44);

    // Tap the row padding, not the nested alias chip.
    await prompt.tap({ position: { x: 4, y: 4 } });
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
    let aroundRequests = 0;
    await testPage.route("**/api/v1/task-sessions/*/messages*", async (route) => {
      const url = new URL(route.request().url());
      if (url.searchParams.get("around")) aroundRequests += 1;
      if (url.searchParams.get("before")) {
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

    const middleRow = panel.locator('[data-testid^="prompt-history-row-"]', {
      hasText: MIDDLE_PROMPT_MARKER,
    });
    await expect(middleRow).toBeAttached({ timeout: 10_000 });

    // The middle prompt remains outside the transcript's initial newest page.
    // Selecting it must load its around-window before Chat attempts to scroll.
    const middlePromptMessageId = await middleRow
      .locator("[data-message-id]")
      .getAttribute("data-message-id");
    if (!middlePromptMessageId) throw new Error("Middle prompt row has no message id");
    await installTargetScrollCounter(testPage, middlePromptMessageId, true);
    await middleRow.locator('[role="button"]').first().tap();
    await expect.poll(() => aroundRequests, { timeout: 10_000 }).toBe(1);
    const targetMessage = new SessionPage(testPage)
      .activeChat()
      .locator(`#msg-${middlePromptMessageId}`);
    await expect(targetMessage).toBeAttached({ timeout: 10_000 });
    await expect
      .poll(
        () =>
          testPage.evaluate(
            () =>
              (window as typeof window & { __promptHistoryDisplacementOffset?: number })
                .__promptHistoryDisplacementOffset ?? Number.NEGATIVE_INFINITY,
          ),
        { timeout: 10_000 },
      )
      .toBeGreaterThan(164);
    await expect
      .poll(
        () =>
          testPage.evaluate(
            () =>
              (
                window as typeof window & {
                  __promptHistoryTargetScrollCalls?: number;
                  __promptHistoryScrollTimes?: number[];
                }
              ).__promptHistoryScrollTimes ?? [],
          ),
        { timeout: 10_000 },
      )
      .toHaveLength(2);
    const scrollTimes = await testPage.evaluate(
      () =>
        (window as typeof window & { __promptHistoryScrollTimes?: number[] })
          .__promptHistoryScrollTimes ?? [],
    );
    expect(scrollTimes[1] - scrollTimes[0]).toBeGreaterThanOrEqual(250);
    expect(scrollTimes[1] - scrollTimes[0]).toBeLessThanOrEqual(1_000);

    const stableTargetOffset = await targetMessage.evaluate(async (element) => {
      const readOffset = () => {
        const list = element.closest(".chat-message-list");
        if (!list) return { offset: Number.POSITIVE_INFINITY, margin: 0 };
        return {
          offset: element.getBoundingClientRect().top - list.getBoundingClientRect().top,
          margin: parseFloat(getComputedStyle(element).scrollMarginTop) || 0,
        };
      };
      const first = readOffset();
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      return { first, second: readOffset() };
    });
    expect(
      Math.abs(stableTargetOffset.second.offset - stableTargetOffset.first.offset),
    ).toBeLessThanOrEqual(1);
    expect(stableTargetOffset.second.offset).toBeGreaterThanOrEqual(-2);
    expect(stableTargetOffset.second.offset).toBeLessThanOrEqual(
      stableTargetOffset.second.margin + 2,
    );

    // No load-more button inside the panel.
    await expect(panel.getByTestId("load-older-messages")).toHaveCount(0);
  });
  test("cancels a mobile unloaded target when Chat is left before reassertion", async ({
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
    const { sessions } = await apiClient.listTaskSessions(taskId);
    const sessionId = sessions[0]?.id;
    if (!sessionId) throw new Error("mobile cancellation task has no session");
    const { messages } = await apiClient.listSessionMessages(sessionId);
    const targetMessage = messages.find((message) => message.content === MIDDLE_PROMPT_MARKER);
    if (!targetMessage) throw new Error("mobile cancellation marker was not persisted");
    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const panelsButton = testPage.getByRole("button", { name: "Panels" });
    await panelsButton.tap();
    await testPage.getByTestId("mobile-prompt-history-option").tap();
    const panel = testPage.getByTestId("prompt-history-panel");
    const targetBubble = panel.locator(`[data-message-id="${targetMessage.id}"]`);
    await revealPromptHistoryTarget(testPage, panel, targetBubble);
    await expect(targetBubble).toBeVisible();
    let releaseAround: (() => void) | null = null;
    const aroundGate = Promise.withResolvers<void>();
    releaseAround = aroundGate.resolve;
    let aroundRequests = 0;
    await testPage.route("**/api/v1/task-sessions/*/messages*", async (route) => {
      const url = new URL(route.request().url());
      if (url.searchParams.get("around") === targetMessage.id) {
        aroundRequests += 1;
        await aroundGate.promise;
      }
      await route.continue();
    });
    await panelsButton.tap();
    await testPage.getByTestId("mobile-prompt-history-option").tap();
    await revealPromptHistoryTarget(testPage, panel, targetBubble);
    await testPage.clock.install();
    await installTargetScrollCounter(testPage, targetMessage.id);
    await targetBubble.tap();
    await expect.poll(() => aroundRequests, { timeout: 10_000 }).toBe(1);
    releaseAround?.();
    await testPage.clock.fastForward(50);
    const chat = session.activeChat();
    await expect(chat.locator(`#msg-${targetMessage.id}`)).toBeAttached();
    await expect
      .poll(
        () =>
          testPage.evaluate(
            () =>
              (window as typeof window & { __promptHistoryTargetScrollCalls?: number })
                .__promptHistoryTargetScrollCalls ?? 0,
          ),
        { timeout: 10_000 },
      )
      .toBe(1);
    await testPage.getByRole("button", { name: "Plan", exact: true }).tap();
    const leavingChatScrollCalls = await testPage.evaluate(
      () =>
        (window as typeof window & { __promptHistoryTargetScrollCalls?: number })
          .__promptHistoryTargetScrollCalls ?? 0,
    );
    await testPage.clock.fastForward(1000);
    await testPage.getByRole("button", { name: "Chat", exact: true }).tap();
    await expect(chat.locator(`#msg-${targetMessage.id}`)).toBeAttached();
    const finalScrollCalls = await testPage.evaluate(
      () =>
        (window as typeof window & { __promptHistoryTargetScrollCalls?: number })
          .__promptHistoryTargetScrollCalls ?? 0,
    );
    expect(finalScrollCalls).toBe(leavingChatScrollCalls);
    expect(aroundRequests).toBe(1);
    await testPage.unroute("**/api/v1/task-sessions/*/messages*");
  });
});
