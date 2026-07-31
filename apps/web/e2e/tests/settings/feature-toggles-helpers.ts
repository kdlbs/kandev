import { expect, type Page } from "@playwright/test";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";

export async function assertUnreadDividerSetting(
  page: Page,
  prCapture: PrAssetCapture,
  projectName: string,
): Promise<void> {
  await page.goto("/settings/system/feature-toggles");

  const unreadDivider = page.getByTestId("feature-toggle-features.unreadDivider");
  await expect(unreadDivider).toBeVisible();
  await expect(unreadDivider.getByText("Unread divider", { exact: true })).toBeVisible();
  await expect(unreadDivider.getByRole("switch", { name: "Toggle Unread divider" })).toBeChecked();
  await expect(unreadDivider).toContainText("Source: Default");
  await expect(unreadDivider).toContainText("Requires restart");

  await unreadDivider.scrollIntoViewIfNeeded();
  await prCapture.screenshot(`unread-divider-setting-${projectName}`, {
    caption: `Unread divider feature toggle (${projectName})`,
    fullPage: true,
  });
}
