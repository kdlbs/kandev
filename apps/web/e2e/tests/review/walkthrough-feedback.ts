import { expect, test, type Locator, type Page } from "@playwright/test";
import type { WsWatcher } from "../../helpers/causal-waits";

// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.1
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.3
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.1
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.1
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.2
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.3
export async function exerciseWalkthroughFeedback(page: Page, card: Locator, ws: WsWatcher) {
  await checkContentSizing(page, card);
  const trigger = card.getByTestId("walkthrough-comment-trigger");
  const picker = card.getByTestId("walkthrough-step-picker");
  await picker.press("ArrowDown");
  await expect(page.getByRole("listbox")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(picker).toBeFocused();
  await expect(card.getByRole("textbox")).toHaveCount(0);
  await trigger.click();
  await card.getByRole("textbox").fill("Saved feedback for the first step");
  await card.getByRole("button", { name: "Add", exact: true }).click();
  await expect(card.getByTestId("walkthrough-comment")).toHaveText(
    /Saved feedback for the first step.*Pending/s,
  );
  await expect(trigger).toContainText("(1)");
  await expect(picker).toContainText("1 comment");
  await trigger.click();
  await card.getByRole("textbox").fill("Draft for step one");
  await selectWalkthroughStep(page, picker, 1);
  await expect(card.getByRole("textbox")).toHaveCount(0);
  await expect(card.getByTestId("walkthrough-comment")).toHaveCount(0);
  await expect(trigger).toContainText("(0)");
  await selectWalkthroughStep(page, picker, 0);
  await expect(card.getByRole("textbox")).toHaveCount(0);
  await trigger.click();
  await expect(card.getByRole("textbox")).toHaveValue("Draft for step one");
  await card.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(trigger).toBeFocused();
  await card.getByTestId("walkthrough-close").click();
  await page.getByTestId("walkthrough-launcher").click();
  await trigger.click();
  await expect(card.getByRole("textbox")).toHaveValue("Draft for step one");
  const admitted = ws.waitForResponse("message.add");
  await card.getByRole("button", { name: "Run", exact: true }).click();
  await admitted;
  await expect(
    card.getByTestId("walkthrough-comment").filter({ hasText: "Draft for step one" }),
  ).toHaveAttribute("data-status", "sent");
  await expect(card.getByRole("textbox")).toHaveCount(0);
  await expect(trigger).toContainText("(2)");
  if ((page.viewportSize()?.width ?? 1280) < 768) {
    for (const control of [trigger, picker, card.getByTestId("walkthrough-next")]) {
      const box = await control.boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(44);
    }
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
}

export async function checkFeedbackReload(page: Page, card: Locator) {
  await page.reload();
  await page.getByTestId("walkthrough-launcher").click();
  await expect(card.getByTestId("walkthrough-comment")).toHaveCount(2);
  await expect(
    card.getByTestId("walkthrough-comment").filter({ hasText: "Draft for step one" }),
  ).toHaveAttribute("data-status", "sent");
  await checkLongFeedback(page, card);
  await page.screenshot({ path: test.info().outputPath("feedback-reading.png") });
  await card.getByTestId("walkthrough-comment-trigger").click();
  await expect(card.getByRole("textbox")).toHaveValue("");
  await page.screenshot({ path: test.info().outputPath("feedback-composing.png") });
  await card.getByRole("button", { name: "Cancel", exact: true }).click();
}

// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.4
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.1
export async function checkFeedbackViewport(page: Page, card: Locator) {
  const original = page.viewportSize()!;
  const draft = "A draft that stays with its step.\n".repeat(20);
  await card.getByTestId("walkthrough-comment-trigger").click();
  await card.getByRole("textbox").fill(draft);
  for (const width of [639, 640, 767, 768]) {
    await page.setViewportSize({ width, height: 600 });
    const launcher = page.getByTestId("walkthrough-launcher");
    await expect(launcher).toBeVisible();
    if (!(await card.isVisible())) await launcher.click();
    await expect(card).toBeVisible();
    const trigger = card.getByTestId("walkthrough-comment-trigger");
    if (await trigger.isVisible()) await trigger.click();
    await expect(card.getByRole("textbox")).toHaveValue(draft);
    const bounds = await card.boundingBox();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width);
    expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(600);
    for (const control of [
      card.getByTestId("walkthrough-step-picker"),
      card.getByRole("button", { name: "Run", exact: true }),
      card.getByTestId("walkthrough-next"),
    ]) {
      const box = await control.boundingBox();
      expect(box!.x).toBeGreaterThanOrEqual(bounds!.x);
      expect(box!.x + box!.width).toBeLessThanOrEqual(bounds!.x + bounds!.width);
      expect(box!.y + box!.height).toBeLessThanOrEqual(bounds!.y + bounds!.height);
      expect(box!.height).toBeGreaterThanOrEqual(width < 768 ? 44 : 27);
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  }
  await page.setViewportSize(original);
  const launcher = page.getByTestId("walkthrough-launcher");
  await expect(launcher).toBeVisible();
  if (!(await card.isVisible())) await launcher.click();
  const trigger = card.getByTestId("walkthrough-comment-trigger");
  if (await trigger.isVisible()) await trigger.click();
  await expect(card.getByRole("textbox")).toHaveValue(draft);
  await card.getByRole("button", { name: "Cancel", exact: true }).click();
}

// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.6
async function checkContentSizing(page: Page, card: Locator) {
  const reading = (await card.boundingBox())!;
  expect(reading.height).toBeLessThan(page.viewportSize()!.height * 0.65);
  await card.getByTestId("walkthrough-comment-trigger").click();
  const composing = (await card.boundingBox())!;
  expect(composing.height).toBeGreaterThan(reading.height);
  await card.getByRole("textbox").fill("A long draft.\n".repeat(30));
  const longDraft = (await card.boundingBox())!;
  expect(longDraft.y + longDraft.height).toBeLessThanOrEqual(page.viewportSize()!.height);
  await card.getByRole("textbox").fill("");
  await card.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect.poll(async () => (await card.boundingBox())!.height).toBe(reading.height);
}

// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.4
async function selectWalkthroughStep(page: Page, picker: Locator, index: number) {
  const mobile = (page.viewportSize()?.width ?? 1280) < 768;
  if (mobile) await picker.tap();
  else await picker.click();
  const option = page.getByRole("option").nth(index);
  await expect(option).toBeVisible();
  if ((page.viewportSize()?.width ?? 1280) < 768) {
    await expect.poll(async () => (await option.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  }
  if (mobile) await option.tap();
  else await option.click();
  await expect(picker).toHaveAttribute("aria-expanded", "false");
}

async function checkLongFeedback(page: Page, card: Locator) {
  const picker = card.getByTestId("walkthrough-step-picker");
  await selectWalkthroughStep(page, picker, 2);
  const shortHeight = (await card.boundingBox())!.height;
  await card.getByTestId("walkthrough-comment-trigger").click();
  await card.getByRole("textbox").fill("A long comment that needs scrolling.\n".repeat(50));
  await card.getByRole("button", { name: "Add", exact: true }).click();
  const bounds = (await card.boundingBox())!;
  expect(bounds.height).toBeGreaterThan(shortHeight);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(page.viewportSize()!.height);
  const scrollBody = card.getByTestId("walkthrough-step-content");
  expect(await scrollBody.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
  await card.getByTestId("walkthrough-next").click();
  expect((await card.boundingBox())!.height).toBeLessThan(bounds.height);
  await selectWalkthroughStep(page, picker, 0);
}
