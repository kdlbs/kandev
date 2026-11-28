import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  expectMermaidMessageUnchanged,
  expectMermaidRecovery,
  expectMermaidDiagramFitsViewport,
  GLUED_MERMAID_MESSAGE,
  seedTaskWithWideMermaidMessage,
} from "./mermaid-fit-helpers";

test.describe("Mobile Mermaid diagram fit", () => {
  test("auto-scales wide diagrams to the mobile chat viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { session } = await seedTaskWithWideMermaidMessage(
      testPage,
      apiClient,
      seedData,
      "Mermaid Fit Mobile",
    );

    await expectMermaidDiagramFitsViewport(session);
    const frame = session.activeChat().locator(".mermaid-block").last();
    const grip = frame.getByRole("separator");
    await grip.scrollIntoViewIfNeeded();
    const box = await grip.boundingBox();
    if (!box) throw new Error("Missing touch resize grip");
    expect(box.height).toBeGreaterThanOrEqual(44);
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    const initial = await frame.evaluate((el) => el.clientHeight);
    const cdp = await testPage.context().newCDPSession(testPage);
    try {
      await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x, y }] });
      await cdp.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x, y: y + 64 }],
      });
      await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
      await expect.poll(() => frame.evaluate((el) => el.clientHeight)).toBe(initial + 64);
      await grip.tap();
      const reset = frame.locator(".mermaid-frame-reset");
      await expect(reset).toBeVisible();
      expect((await reset.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      await reset.tap();
      await expect.poll(() => frame.evaluate((el) => el.clientHeight)).toBe(initial);
    } finally {
      await cdp.detach();
    }
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
      "Mermaid Fence Recovery Mobile",
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
