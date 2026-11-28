import { expect, type Locator } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  expectMermaidMessageUnchanged,
  expectMermaidRecovery,
  expectMermaidDiagramFitsViewport,
  GLUED_MERMAID_MESSAGE,
  seedTaskWithWideMermaidMessage,
} from "./mermaid-fit-helpers";

async function expectZoomInsideFrame(region: Locator) {
  await region.scrollIntoViewIfNeeded();
  const box = await region.boundingBox();
  if (!box) throw new Error("Missing diagram viewport");
  const svg = region.locator("svg").first();
  const width = () => svg.evaluate((el) => el.getBoundingClientRect().width);
  const initialWidth = await width();
  const pageScale = () => region.page().evaluate(() => window.visualViewport?.scale);
  const initialPageScale = await pageScale();
  const cdp = await region.page().context().newCDPSession(region.page());
  try {
    await cdp.send("Input.synthesizePinchGesture", {
      x: box.x + box.width / 2,
      y: box.y + box.height / 2,
      scaleFactor: 1.5,
      gestureSourceType: "mouse",
    });
    await expect.poll(width).toBeGreaterThan(initialWidth);
    const zoomedFrame = await region.boundingBox();
    expect(zoomedFrame?.height).toBeCloseTo(box.height, 0);
    expect(zoomedFrame?.width).toBeCloseTo(box.width, 0);
    expect(await pageScale()).toBe(initialPageScale);
  } finally {
    await cdp.detach();
  }
}

async function expectManualResize(frame: Locator) {
  const grip = frame.getByRole("separator");
  await grip.scrollIntoViewIfNeeded();
  const box = await grip.boundingBox();
  if (!box) throw new Error("Missing diagram resize grip");
  const height = () => frame.evaluate((el) => el.getBoundingClientRect().height);
  const width = () =>
    frame
      .locator("svg")
      .first()
      .evaluate((el) => el.getBoundingClientRect().width);
  const initial = await height();
  const art = await width();
  const mouse = frame.page().mouse;
  await mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await mouse.down();
  await mouse.move(box.x + box.width / 2, box.y + box.height / 2 + 80);
  await mouse.up();
  expect(await height()).toBeCloseTo(initial + 80, 0);
  await grip.press("ArrowDown");
  expect(await height()).toBeCloseTo(initial + 96, 0);
  expect(await width()).toBe(art);
  await grip.press("Enter");
  expect(await height()).toBeCloseTo(initial, 0);
}

test.describe("Mermaid diagram fit", () => {
  test("fits wide diagrams and confines trackpad zoom to the diagram", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { session, taskId } = await seedTaskWithWideMermaidMessage(
      testPage,
      apiClient,
      seedData,
      "Mermaid Fit Desktop",
    );

    await expectMermaidDiagramFitsViewport(session);

    const region = session.activeChat().locator(".mermaid-scroll-region").last();
    await expectZoomInsideFrame(region);
    await expectManualResize(region.locator(".."));

    await apiClient.wsRequest("task.plan.create", {
      task_id: taskId,
      title: "Plan diagram",
      content: ["```mermaid", "flowchart LR", "A --> B", "```"].join("\n"),
    });
    if (!(await session.planPanel.isVisible())) await session.togglePlanMode();
    const planRegion = session.planPanel.locator(".mermaid-container").first();
    await expect(planRegion.locator("div svg").first()).toBeVisible();
    const planWidth = await planRegion.evaluate((el) => {
      const parent = el.parentElement!;
      const style = getComputedStyle(parent);
      return parent.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight);
    });
    await expect
      .poll(() => planRegion.getByRole("separator").evaluate((el) => el.clientWidth))
      .toBeCloseTo(planWidth, 0);
    await expectZoomInsideFrame(planRegion);
    const revisions = () => apiClient.wsRequest("task.plan.revisions.list", { task_id: taskId });
    const before = await revisions();
    const content = (await apiClient.getTaskPlan(taskId))?.content;
    await expectManualResize(planRegion);
    expect((await apiClient.getTaskPlan(taskId))?.content).toBe(content);
    expect(await revisions()).toEqual(before);
  });

  // @covers AC-UI-MERMAID-RENDERING-002.4, AC-UI-MERMAID-RENDERING-002.5
  test("separates glued prose and preserves the saved message after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const seeded = await seedTaskWithWideMermaidMessage(
      testPage,
      apiClient,
      seedData,
      "Mermaid Fence Recovery Desktop",
      GLUED_MERMAID_MESSAGE,
    );

    await expectMermaidRecovery(seeded.session);
    await expectMermaidMessageUnchanged(apiClient, seeded.sessionId, GLUED_MERMAID_MESSAGE);

    await testPage.reload();
    await seeded.session.waitForLoad();
    await expectMermaidRecovery(seeded.session);
    await expectMermaidMessageUnchanged(apiClient, seeded.sessionId, GLUED_MERMAID_MESSAGE);
  });
});
