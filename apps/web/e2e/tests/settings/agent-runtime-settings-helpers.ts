import { expect, type Page } from "@playwright/test";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.5, AC-AGENTS-RUNTIME-NOTIFY-001.6
export async function compactRuntimeSettings(
  page: Page,
  mobile: boolean,
  capture?: PrAssetCapture,
) {
  await page.goto("/settings/agents");
  const section = page.locator("#runtime-updates");
  const disclosure = section.locator(":scope > details");
  const summary = disclosure.locator(":scope > summary");
  const managed = page.getByTestId("runtime-policy-claude-acp");
  await expect(page.locator("#installed-agent-claude-acp")).toBeVisible();
  await expect(disclosure).not.toHaveAttribute("open");
  await expect(managed).not.toBeVisible();
  expect(
    await section.evaluate((element) =>
      Boolean(
        document.getElementById("installed-agent-claude-acp")!.compareDocumentPosition(element) &
        Node.DOCUMENT_POSITION_FOLLOWING,
      ),
    ),
  ).toBe(true);
  await summary.scrollIntoViewIfNeeded();
  if (mobile) expect((await summary.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await capture?.screenshot("runtime-settings-collapsed", {
    caption:
      "Runtime policy stays collapsed after the installed agents on ordinary Settings entry.",
  });
  if (mobile && (await page.evaluate(() => matchMedia("(pointer:coarse)").matches)))
    await summary.tap();
  else {
    await summary.focus();
    await summary.press("Enter");
  }
  await expect(managed).toBeVisible();
  await expect(summary.locator("svg")).toHaveCSS("rotate", "90deg");
  const policy = managed.locator('[data-settings-touch-target="true"]');
  const action = managed.getByRole("link", { name: "Manage versions", exact: true });
  if (mobile) {
    expect((await policy.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    expect((await action.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await expect(policy).toHaveCSS("width", "44px");
  } else {
    expect((await action.boundingBox())!.height).toBeCloseTo(28, 0);
  }
  if (capture?.capturing) {
    await summary.evaluate((element) => {
      element.scrollIntoView({ block: "start" });
      for (let ancestor = element.parentElement; ancestor; ancestor = ancestor.parentElement) {
        if (["auto", "scroll"].includes(getComputedStyle(ancestor).overflowY)) {
          ancestor.scrollBy({ top: -64 });
          break;
        }
      }
    });
    await expect(summary).toBeInViewport();
    await expect(action).toBeInViewport();
  }
  await capture?.screenshot("runtime-settings-expanded", {
    caption:
      "Compact runtime rows share help and keep policy controls and update guidance available.",
  });
  await summary.click();
  await expect(managed).not.toBeVisible();
  await expect(summary.locator("svg")).toHaveCSS("rotate", "none");
  await page.evaluate(() => {
    window.location.hash = "runtime-update-claude-acp";
  });
  await expect(managed).toBeVisible();
  await expect(disclosure).toHaveAttribute("open");
  await expect(managed.getByRole("switch", { name: "Automatic updates" })).toBeFocused();
  await expect(async () => {
    const box = (await managed.boundingBox())!;
    expect(box.y).toBeGreaterThanOrEqual(0);
    expect(box.y + box.height).toBeLessThanOrEqual(page.viewportSize()!.height);
  }).toPass();
  await page.goto("/settings/agents#runtime-update-disabled-cli");
  const inactive = page.getByTestId("runtime-policy-disabled-cli");
  await expect(inactive).toBeVisible();
  await expect(inactive).toContainText("Unavailable or disabled");
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
}

export async function retainRuntimePolicyDraft(page: Page) {
  const section = page.locator("#runtime-updates");
  const summary = section.locator(":scope > details > summary");
  const automatic = page
    .getByTestId("runtime-policy-claude-acp")
    .getByRole("switch", { name: "Automatic updates" });
  await summary.click();
  await expect(automatic).not.toBeVisible();
  await expect(section).toHaveAttribute("data-settings-dirty", "true");
  await summary.click();
  await expect(automatic).toBeChecked();
}
