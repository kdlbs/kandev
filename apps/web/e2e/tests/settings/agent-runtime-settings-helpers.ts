import { expect, type Locator, type Page } from "@playwright/test";
import { waitForFiniteAnimations } from "../../helpers/animations";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";

export async function expectRuntimeVersions(surface: Locator, current: string, target: string) {
  const summary = surface.locator('[data-testid^="agent-update-version-summary-"]');
  if (current !== target)
    await expect(summary.getByText(`Current version ${current}`, { exact: true })).toBeVisible();
  await expect(summary.getByText(target, { exact: true })).toBeVisible();
}

export async function chooseKandevRuntimeDefault(page: Page, agentName: string) {
  await page.getByTestId(`agent-update-version-${agentName}`).click();
  await page.getByTestId(`agent-update-version-default-${agentName}`).click();
}

async function expectBackBesideTitle(surface: Locator, back: Locator, mobile: boolean) {
  await waitForFiniteAnimations(surface);
  const title = surface.locator('[data-slot="dialog-title"], [data-slot="drawer-title"]');
  await expect(back).toHaveAccessibleName("Back to all runtimes");
  await expect(back).toBeInViewport({ ratio: 1 });
  const [backBox, titleBox] = await Promise.all([back.boundingBox(), title.boundingBox()]);
  const { x, y, height, width } = backBox!;
  expect(x + width).toBeLessThanOrEqual(titleBox!.x);
  expect(Math.abs(y + height / 2 - (titleBox!.y + titleBox!.height / 2))).toBeLessThanOrEqual(2);
  if (mobile) expect(Math.min(width, height)).toBeGreaterThanOrEqual(44);
  else expect(Math.max(width, height)).toBeLessThan(44);
}

async function expectCompactRuntimeDetail(page: Page, detail: Locator, touch: boolean) {
  await expect(detail.getByText("Quick choices", { exact: true })).toBeHidden();
  const latest = detail.getByText("Latest version", { exact: true });
  const automatic = detail.getByText("Automatic updates", { exact: true });
  const runtimeDetails = detail.getByTestId(`agent-update-runtime-details-claude-acp`);
  const runtimeDetailsSummary = runtimeDetails.locator(":scope > summary");
  const guidance = detail.getByRole("link", { name: "Manual update guidance" });
  const confirm = detail.getByRole("button", { name: "Update to 0.64.0" });
  await expect(detail.getByRole("button", { name: "Change version" })).toBeVisible();
  await expect(guidance).toBeHidden();
  await expect(latest).toBeVisible();
  await expect(runtimeDetails).toBeVisible();
  await expect(confirm).toBeEnabled();
  const [versionBox, policyBox, detailsBox] = await Promise.all([
    latest.boundingBox(),
    automatic.boundingBox(),
    runtimeDetails.boundingBox(),
  ]);
  expect(Math.abs(versionBox!.x - policyBox!.x)).toBeLessThan(1);
  expect(Math.abs(versionBox!.x - detailsBox!.x)).toBeLessThan(1);
  if (touch) {
    await runtimeDetailsSummary.tap();
  } else {
    const versionPicker = detail.getByTestId("agent-update-version-claude-acp");
    await versionPicker.click();
    await page.keyboard.press("Escape");
    await expect(versionPicker).toBeFocused();
    for (let index = 0; index < 24; index += 1) {
      if (await runtimeDetailsSummary.evaluate((element) => element === document.activeElement))
        break;
      await page.keyboard.press("Tab");
    }
    await expect(runtimeDetailsSummary).toBeFocused();
    await page.keyboard.press("Enter");
  }
  await expect(guidance).toBeVisible();
  const command = detail.getByTestId("agent-update-command-claude-acp");
  const commandSummary = command.locator(":scope > summary");
  await expect(commandSummary).toBeVisible();
  await expect(command.locator("pre")).toBeHidden();
  if (touch) {
    await commandSummary.tap();
  } else {
    await page.keyboard.press("Tab");
    await expect(commandSummary).toBeFocused();
    await page.keyboard.press("Enter");
  }
  await expect(command.locator("pre")).toContainText("@agentclientprotocol/claude-agent-acp");
  await expect(command.locator("pre")).toBeVisible();
  await expect(confirm).toBeInViewport();
}

// @covers AC-AGENTS-RUNTIME-MANAGER-001.1, AC-AGENTS-RUNTIME-MANAGER-001.4, AC-AGENTS-RUNTIME-MANAGER-001.6
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
  const touch = mobile && (await page.evaluate(() => matchMedia("(pointer:coarse)").matches));
  if (touch) await entry.tap();
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

  await expectCompactRuntimeDetail(page, detail, touch);
  const back = detail.getByTestId("agent-update-back-claude-acp");
  await expectBackBesideTitle(detail, back, mobile);
  if (touch) await back.tap();
  else await back.click();
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
  const guidanceBack = inactive.getByTestId("runtime-guidance-back");
  await expectBackBesideTitle(inactive, guidanceBack, mobile);
  if (touch) await guidanceBack.tap();
  else await guidanceBack.click();
  await expect(overview).toBeVisible();
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
