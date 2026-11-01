import fs from "node:fs";
import path from "node:path";
import { expect, type Locator, type Page } from "@playwright/test";
import { RICH_OUTPUT_IMAGE, seedRichOutputImage } from "./rich-output-helpers";

export async function seedSizedImage(
  page: Page,
  repositoryPath: string,
  size: { width: number; height: number },
) {
  const payload = seedRichOutputImage(repositoryPath);
  const data = await page.evaluate(({ width, height }) => {
    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;
    const context = canvas.getContext("2d")!;
    context.fillStyle = "#3878ac";
    context.fillRect(0, 0, width, height);
    return canvas.toDataURL("image/png").split(",")[1];
  }, size);
  fs.writeFileSync(path.join(repositoryPath, RICH_OUTPUT_IMAGE), Buffer.from(data, "base64"));
  return payload;
}

export async function expectTouchTarget(control: Locator) {
  const box = (await control.boundingBox())!;
  expect(box.width).toBeGreaterThanOrEqual(44);
  expect(box.height).toBeGreaterThanOrEqual(44);
}

export async function expectImageDialogContained(page: Page) {
  const dialog = page.getByRole("dialog", { name: "Image preview" });
  await expect(dialog).toBeVisible();
  const image = dialog.getByRole("img");
  await expect
    .poll(() => image.evaluate((img: HTMLImageElement) => img.naturalWidth))
    .toBeGreaterThan(0);
  const close = dialog.getByRole("button", { name: "Close", exact: true });
  const viewport = page.viewportSize()!;
  for (const element of [dialog, image, close]) {
    const box = (await element.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.y).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(viewport.width);
    expect(box.y + box.height).toBeLessThanOrEqual(viewport.height);
  }
  const dialogBox = (await dialog.boundingBox())!;
  const closeBox = (await close.boundingBox())!;
  expect(closeBox.x).toBeGreaterThanOrEqual(dialogBox.x);
  expect(closeBox.y).toBeGreaterThanOrEqual(dialogBox.y);
  expect(closeBox.x + closeBox.width).toBeLessThanOrEqual(dialogBox.x + dialogBox.width);
  expect(closeBox.y + closeBox.height).toBeLessThanOrEqual(dialogBox.y + dialogBox.height);
  await expectTouchTarget(close);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  return close;
}
