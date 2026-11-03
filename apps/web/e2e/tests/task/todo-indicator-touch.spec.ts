import { test, expect } from "../../fixtures/test-base";
import { expectTodoDisclosure, openTodoTask } from "../../helpers/todo-disclosure";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";

// @covers AC-UI-AGENT-TODO-LIST-PANEL-002.1 and .3
test("wide coarse desktop opens session todos with CDP touch", async ({
  coarseDesktopTestPage: page,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(90_000);
  const capture = new PrAssetCapture(page, testInfo.file, { captureKey: "wide" });
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Emulation.setTouchEmulationEnabled", { enabled: true, maxTouchPoints: 1 });
  const trigger = await openTodoTask(page, apiClient, seedData);
  expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
  const box = await trigger.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeGreaterThanOrEqual(44);
  expect(box!.height).toBeGreaterThanOrEqual(44);
  const point = { x: box!.x + box!.width / 2, y: box!.y + box!.height / 2 };
  await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [point] });
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  const content = await expectTodoDisclosure(page, "touch");
  await capture.screenshot("wide-coarse-todos", {
    page,
    caption: "Wide touch device showing the session checklist",
  });
  capture.flush();
  await page.keyboard.press("Escape");
  await expect(content).toBeHidden();
  await expect(trigger).toBeFocused();
});

test("fine desktop retains hover and keyboard focus disclosure", async ({
  testPage: page,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(90_000);
  await page.setViewportSize({ width: 1280, height: 900 });
  const trigger = await openTodoTask(page, apiClient, seedData);
  expect(await page.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
  await trigger.hover();
  const content = await expectTodoDisclosure(page, "mouse");
  await prCapture.screenshot("fine-desktop-todos", {
    caption: "Desktop hover showing the session checklist",
  });
  await page.mouse.move(0, 0);
  await expect(content).toBeHidden();
  await trigger.focus();
  await expect(content).toBeVisible();
});
