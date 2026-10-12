import { expect, type Page } from "@playwright/test";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

export async function assertDeliveryRecoveryGeometry(page: Page) {
  const retry = page.getByTestId("recovery-retry-connection-button");
  const stop = page.getByTestId("recovery-stop-button");
  await expect(retry).toBeVisible();
  await expect(stop).toBeVisible();
  await stop.scrollIntoViewIfNeeded();
  const retryBox = await retry.boundingBox();
  const stopBox = await stop.boundingBox();
  expect(retryBox).not.toBeNull();
  expect(stopBox).not.toBeNull();
  const viewportHeight = page.viewportSize()!.height;
  expect(retryBox!.y).toBeGreaterThanOrEqual(0);
  expect(retryBox!.y + retryBox!.height).toBeLessThanOrEqual(viewportHeight + 1);
  expect(stopBox!.y).toBeGreaterThanOrEqual(0);
  expect(stopBox!.y + stopBox!.height).toBeLessThanOrEqual(viewportHeight + 1);
  const touch = await page.evaluate(() => matchMedia("(pointer: coarse)").matches);
  const narrow = page.viewportSize()!.width < 768;
  if (touch || narrow) {
    expect(retryBox!.height).toBeGreaterThanOrEqual(44);
    expect(stopBox!.height).toBeGreaterThanOrEqual(44);
  }
  if (!narrow) {
    expect(Math.abs(retryBox!.y - stopBox!.y)).toBeLessThanOrEqual(1);
    expect(Math.abs(retryBox!.height - stopBox!.height)).toBeLessThanOrEqual(1);
  }
  await assertNoDocumentHorizontalOverflow(page, "delivery recovery controls");
}
