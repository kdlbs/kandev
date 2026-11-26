import { test, expect } from "../../fixtures/test-base";
import { openAgentReferences } from "./agent-task-reference-helpers";

// @covers AC-UI-AGENT-TASK-REFERENCE-001.1, AC-UI-AGENT-TASK-REFERENCE-001.2
// @covers AC-UI-AGENT-TASK-REFERENCE-001.4
test("phone agent references wrap and open with one tap", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const { target, source, message } = await openAgentReferences(testPage, apiClient, seedData);
  const links = message.getByTestId("task-mention-chip");
  await expect(links).toHaveCount(2);
  const targetLink = message.locator(`a[href="/t/${target.id}"]`);
  const accent = await targetLink.evaluate((element) => getComputedStyle(element).color);
  await apiClient.saveUserSettings({
    sidebar_task_color_patch: { colors: { [target.id]: "green" } },
  });
  await expect
    .poll(() => targetLink.evaluate((element) => getComputedStyle(element).color))
    .not.toBe(accent);
  await expect(targetLink).toHaveCSS("min-height", "24px");
  const labelHeight = await targetLink
    .locator("span")
    .evaluate((el) => el.getBoundingClientRect().height);
  expect((await targetLink.boundingBox())!.height).toBeLessThanOrEqual(
    Math.max(24, labelHeight + 6) + 1,
  );
  const bounds = await message.boundingBox();
  expect(bounds).not.toBeNull();
  const boxes = await Promise.all((await links.all()).map((link) => link.boundingBox()));
  for (const box of boxes) {
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(24);
    expect(box!.x).toBeGreaterThanOrEqual(bounds!.x);
    expect(box!.x + box!.width).toBeLessThanOrEqual(bounds!.x + bounds!.width);
  }
  expect(boxes[1]!.y).toBeGreaterThanOrEqual(boxes[0]!.y + boxes[0]!.height);
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(
    false,
  );
  await prCapture.screenshot("phone-agent-task-references", {
    caption: "Wrapping task references with direct navigation",
  });
  await message.locator(`a[href="/t/${target.id}"]`).tap();
  await expect(testPage).toHaveURL(new RegExp(`/t/${target.id}(?:\\?|$)`));
  await expect(testPage.getByRole("tooltip")).toHaveCount(0);
  await testPage.goBack();
  await expect(testPage).toHaveURL(new RegExp(`/t/${source.id}(?:\\?|$)`));
});
