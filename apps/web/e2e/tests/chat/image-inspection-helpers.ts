import { expect, type Page, type Locator } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { seedSizedImage } from "./rich-output-image-helpers";
import { RICH_OUTPUT_IMAGE, seedRichOutputTask } from "./rich-output-helpers";
import path from "node:path";

export async function openInspection(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  phone: boolean,
) {
  const payload = await seedSizedImage(page, seedData.repositoryPath, {
    width: 2400,
    height: 1800,
  });
  const session = await seedRichOutputTask({
    page,
    apiClient,
    seedData,
    title: "Image inspection",
    payload,
  });
  const card = session
    .activeChat()
    .getByTestId("rich-output-file")
    .filter({ hasText: RICH_OUTPUT_IMAGE });
  const trigger = card.getByRole("button", { name: `Open ${RICH_OUTPUT_IMAGE}`, exact: true });
  await expect(trigger).toBeVisible();
  const source = await trigger.locator("img").getAttribute("src");
  if (phone) await trigger.tap();
  else await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Image preview" });
  const image = dialog.getByRole("img");
  await expect(image).toHaveAttribute("src", source!);
  await expect.poll(() => image.evaluate((el: HTMLImageElement) => el.naturalWidth)).toBe(2400);
  expect((await image.boundingBox())!.width).toBeLessThan(2400);
  return { session, trigger, dialog, image, source };
}

export async function expectActualGeometry(dialog: Locator) {
  const image = dialog.getByRole("img");
  const canvas = dialog.getByRole("region", { name: "Image inspection" });
  const size = await image.evaluate((el: HTMLImageElement) => ({
    width: el.getBoundingClientRect().width,
    height: el.getBoundingClientRect().height,
    naturalWidth: el.naturalWidth,
    naturalHeight: el.naturalHeight,
  }));
  expect(size.width).toBeCloseTo(size.naturalWidth, 0);
  expect(size.height).toBeCloseTo(size.naturalHeight, 0);
  const overflow = await canvas.evaluate((el) => ({
    x: el.scrollWidth - el.clientWidth,
    y: el.scrollHeight - el.clientHeight,
  }));
  expect(overflow.x).toBeGreaterThan(0);
  expect(overflow.y).toBeGreaterThan(0);
  return canvas;
}

export async function touchScroll(page: Page, canvas: Locator, axis: "x" | "y") {
  const box = (await canvas.boundingBox())!;
  const cdp = await page.context().newCDPSession(page);
  const start = { x: box.x + box.width * 0.75, y: box.y + box.height * 0.75 };
  const end =
    axis === "x"
      ? { x: box.x + box.width * 0.25, y: start.y }
      : { x: start.x, y: box.y + box.height * 0.25 };
  await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [start] });
  for (let i = 1; i <= 6; i++) {
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [
        { x: start.x + ((end.x - start.x) * i) / 6, y: start.y + ((end.y - start.y) * i) / 6 },
      ],
    });
  }
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await cdp.detach();
  await expect
    .poll(() => canvas.evaluate((el, key) => (key === "x" ? el.scrollLeft : el.scrollTop), axis))
    .toBeGreaterThan(0);
  await canvas.evaluate(async (el) => {
    let stableFrames = 0;
    let previous = [el.scrollLeft, el.scrollTop];
    while (stableFrames < 6) {
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      const current = [el.scrollLeft, el.scrollTop];
      stableFrames =
        current[0] === previous[0] && current[1] === previous[1] ? stableFrames + 1 : 0;
      previous = current;
    }
  });
}

export async function exerciseComposer(page: Page, repositoryPath: string, phone: boolean) {
  const input = page.getByTestId("chat-input-editor-shell").locator('input[type="file"]');
  await input.setInputFiles(path.join(repositoryPath, RICH_OUTPUT_IMAGE));
  const opener = page.getByRole("button", { name: /^Image \(/ });
  await expect(opener).toHaveCount(1);
  const open = async () => {
    if (phone) {
      await opener.tap();
      await page.getByRole("button", { name: "Open", exact: true }).tap();
    } else {
      await opener.click();
    }
  };
  await open();
  const dialog = page.getByRole("dialog", { name: "Image preview" });
  await expect(dialog.getByRole("button", { name: "Fit", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  if (phone) await dialog.getByRole("button", { name: "Actual size", exact: true }).tap();
  else await dialog.getByRole("button", { name: "Actual size", exact: true }).click();
  await expectActualGeometry(dialog);
  if (phone) await dialog.getByRole("button", { name: "Close", exact: true }).tap();
  else await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(opener).toBeFocused();
  await open();
  await expect(dialog.getByRole("button", { name: "Fit", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  if (phone) await dialog.getByRole("button", { name: "Close", exact: true }).tap();
  else await dialog.getByRole("button", { name: "Close", exact: true }).click();
}
