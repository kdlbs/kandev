import { expect, type Page } from "@playwright/test";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";

// @covers AC-AGENTS-RUNTIME-MANAGER-001.1, AC-AGENTS-RUNTIME-MANAGER-001.6
export async function compactRuntimeSettings(
  page: Page,
  mobile: boolean,
  capture?: PrAssetCapture,
) {
  await page.goto("/settings/agents");
  const entry = page.getByTestId("runtime-updates-button");
  await expect(entry).toBeVisible();
  await expect(page.getByTestId("runtime-updates-count")).toHaveText("1");
  if (mobile) expect((await entry.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await capture?.screenshot("runtime-settings-entry", {
    caption: "Runtime updates stays visible above installed agents on ordinary Settings entry.",
  });
  if (mobile && (await page.evaluate(() => matchMedia("(pointer:coarse)").matches)))
    await entry.tap();
  else await entry.click();

  const overview = page.getByTestId(mobile ? "runtime-manager-drawer" : "runtime-manager-dialog");
  await expect(overview).toBeVisible();
  await expect(overview).toContainText("Available updates");
  const managed = overview.getByTestId("runtime-manager-row-claude-acp");
  await expect(managed).toBeVisible();
  await expect(page.getByTestId("runtime-updates-count")).toHaveCount(1);
  if (capture?.capturing) {
    await expect(overview).toBeInViewport();
    await capture.screenshot("runtime-settings-expanded", {
      caption:
        "The runtime manager groups available updates first and keeps individual actions visible.",
    });
  }

  const action = page.getByTestId("runtime-manage-versions-claude-acp");
  if (mobile) expect((await action.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  else expect((await action.boundingBox())!.height).toBeGreaterThanOrEqual(26);
  await action.click();

  const detail = page.getByTestId(`agent-update-${mobile ? "drawer" : "dialog"}-claude-acp`);
  await expect(detail).toBeVisible();
  const automatic = detail.getByRole("switch", { name: "Automatic updates" });
  await expect(automatic).toBeVisible();
  if (mobile)
    expect(
      (await page.getByTestId("runtime-automatic-target-claude-acp").boundingBox())!.height,
    ).toBeGreaterThanOrEqual(43.5);
  await expect(page.getByTestId("runtime-updates-count")).toHaveText("1");

  await page.getByTestId("agent-update-back-claude-acp").click();
  await expect(overview).toBeVisible();
  await page.getByTestId("runtime-manager-close").click();
  await expect(overview).toHaveCount(0);

  await page.goto("/settings/agents#runtime-update-disabled-cli");
  const inactive = page.getByTestId(
    `runtime-guidance-${mobile ? "drawer" : "dialog"}-disabled-cli`,
  );
  await expect(inactive).toBeVisible();
  await expect(inactive).toContainText("Unavailable or disabled");
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
}

export async function retainRuntimePolicyDraft(page: Page) {
  const entry = page.getByTestId("runtime-updates-button");
  await entry.click();
  await page.getByTestId("runtime-manage-versions-claude-acp").click();
  const automatic = page
    .getByTestId("agent-update-dialog-claude-acp")
    .getByRole("switch", { name: "Automatic updates" });
  await expect(automatic).not.toBeChecked();
  await automatic.click();
  await expect(page.getByTestId("runtime-policy-dirty-footer")).toBeVisible();
  await page.getByTestId("agent-update-back-claude-acp").click();
  await expect(page.getByTestId("runtime-policy-dirty-footer")).toBeVisible();
  await page.getByTestId("runtime-manager-close").click();
  await entry.click();
  await page.getByTestId("runtime-manage-versions-claude-acp").click();
  await expect(automatic).toBeChecked();
}
