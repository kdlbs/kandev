import { type CDPSession, type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { dwell } from "../../helpers/causal-waits";

test.setTimeout(120_000);

async function swipeCard(page: Page, cdp: CDPSession, scroll: Locator, hold: boolean) {
  const start = await scroll.evaluate((element) => {
    const viewport = element.getBoundingClientRect();
    const cards = Array.from(element.querySelectorAll<HTMLElement>("[data-kanban-card]"));
    const card = cards.find((candidate) => {
      const box = candidate.getBoundingClientRect();
      return box.top > viewport.top + viewport.height / 2 && box.bottom < viewport.bottom;
    });
    if (!card) throw new Error("No fully visible card in the lower half of the scroll area");
    const box = card.getBoundingClientRect();
    return { x: box.x + box.width / 2, y: box.y + box.height / 2, distance: viewport.height / 3 };
  });
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: start.x, y: start.y }],
  });
  if (hold) {
    await dwell(
      page,
      350,
      "library-timer",
      "Exceed the former 250ms card drag activation delay before swiping",
    );
  }
  for (let step = 1; step <= 10; step++) {
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: start.x, y: start.y - (start.distance * step) / 10 }],
    });
  }
  await expect(page.locator('[data-testid^="mobile-drop-target-"]')).toHaveCount(0);
  await expect(page.locator('[data-kanban-card][aria-grabbed="true"]')).toHaveCount(0);
  await expect(page.locator('[data-testid^="kanban-insertion-indicator-"]')).toHaveCount(0);
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
}

for (const width of [393, 767]) {
  for (const hold of [false, true]) {
    // @covers AC-TASKS-MOBILE-KANBAN-SCROLL-001.1, .2, .3
    test(`card swipes scroll at ${width}px without moving tasks${hold ? " after a hold" : ""}`, async ({
      testPage,
      apiClient,
      seedData,
    }, testInfo) => {
      const tasks = [];
      for (let index = 0; index < 24; index++) {
        tasks.push(
          await apiClient.createTask(seedData.workspaceId, `Scroll task ${index}`, {
            workflow_id: seedData.workflowId,
            workflow_step_id: seedData.startStepId,
          }),
        );
      }
      const mobile = new MobileKanbanPage(testPage);
      await testPage.setViewportSize({ width, height: 851 });
      await mobile.goto();
      const scroll = mobile
        .mobileKanbanLayout()
        .getByTestId(`kanban-column-${seedData.startStepId}`)
        .getByTestId("kanban-column-scroll");
      await expect(scroll).toBeVisible();
      const cdp = await testPage.context().newCDPSession(testPage);
      const mutations: string[] = [];
      testPage.on("request", (request) => {
        if (
          (request.method() === "PUT" &&
            /\/workflow-steps\/.+\/tasks\/reorder$/.test(request.url())) ||
          (request.method() === "POST" && /\/tasks\/[^/]+\/move$/.test(request.url()))
        ) {
          mutations.push(request.url());
        }
      });
      await expect(mobile.taskCard(tasks[0].id)).toBeVisible();
      await expect
        .poll(() =>
          scroll.evaluate(
            (element) => element.scrollHeight - element.clientHeight - element.scrollTop,
          ),
        )
        .toBeGreaterThan(100);
      const beforeSwipe = await scroll.evaluate((element) => element.scrollTop);
      await swipeCard(testPage, cdp, scroll, hold);
      await expect
        .poll(() => scroll.evaluate((element) => element.scrollTop))
        .toBeGreaterThan(beforeSwipe + 40);
      await expect(testPage).toHaveURL(/\/$/);
      expect(mutations).toEqual([]);
      const lastCard = mobile.taskCard(tasks.at(-1)!.id);
      for (let attempt = 0; attempt < 20; attempt++) {
        const reachedBottom = await scroll.evaluate(
          (element) => element.scrollTop + element.clientHeight >= element.scrollHeight - 2,
        );
        if (reachedBottom) break;
        await swipeCard(testPage, cdp, scroll, false);
      }
      await expect(lastCard).toBeInViewport();
      for (const task of tasks) {
        const persisted = await apiClient.getTask(task.id);
        expect(persisted.position).toBe(task.position);
        expect(persisted.workflow_step_id).toBe(seedData.startStepId);
      }
      await testInfo.attach("phone-scrolled-cards", {
        body: await testPage.screenshot({ path: testInfo.outputPath("phone-scrolled-cards.png") }),
        contentType: "image/png",
      });
      expect(mutations).toEqual([]);
      await lastCard.tap();
      await expect(testPage).toHaveURL(new RegExp(`/t/${tasks.at(-1)!.id}`));
      await cdp.detach();
    });
  }
}

// @covers AC-TASKS-MOBILE-KANBAN-SCROLL-001.4
test("resizing switches drag affordances at the phone boundary", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Responsive card", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  for (const width of [767, 768, 767]) {
    await testPage.setViewportSize({ width, height: 851 });
    const card = mobile.taskCard(task.id);
    await expect(card).toBeVisible();
    await expect(card).toHaveCSS("touch-action", "auto");
    if (width < 768) {
      await expect(card).not.toHaveAttribute("aria-roledescription", "draggable");
      await expect(card).not.toHaveAttribute("aria-describedby");
    } else {
      await expect(card).toHaveAttribute("aria-roledescription", "draggable");
    }
  }
  const initialStep = await mobile.boardNavigator.textContent();
  const cardBox = await mobile.taskCard(task.id).boundingBox();
  if (!cardBox) throw new Error("Card has no layout box");
  const cdp = await testPage.context().newCDPSession(testPage);
  const y = cardBox.y + cardBox.height / 2;
  const x = cardBox.x + cardBox.width * 0.8;
  await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x, y }] });
  for (let step = 1; step <= 10; step++) {
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: x - (cardBox.width * 0.7 * step) / 10, y }],
    });
  }
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await expect.poll(() => mobile.boardNavigator.textContent()).not.toBe(initialStep);
  expect((await apiClient.getTask(task.id)).workflow_step_id).toBe(seedData.startStepId);
  await cdp.detach();
});
