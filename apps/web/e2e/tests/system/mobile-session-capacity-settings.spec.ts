import { expect, test } from "../../fixtures/test-base";
import { expectElementsNotToIntersect } from "../../helpers/layout-assertions";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import type { SessionCapacitySettingsValue } from "../../../lib/types/system";
import {
  requestSessionCapacitySettings,
  restoreSessionCapacitySettings,
  SESSION_CAPACITY_SETTINGS_PATH,
} from "../../helpers/session-capacity-settings";

let baseline: SessionCapacitySettingsValue | undefined;

test.beforeEach(async ({ apiClient }) => {
  baseline = (await requestSessionCapacitySettings(apiClient, "GET")).settings;
  await requestSessionCapacitySettings(apiClient, "PATCH", { enabled: false });
});

test.afterEach(async ({ apiClient }) => {
  if (!baseline) return;
  await restoreSessionCapacitySettings(apiClient, baseline);
  baseline = undefined;
});

test("reaches Task Behavior and keeps the form touch-safe", async ({ testPage }) => {
  if (!baseline) throw new Error("session capacity settings baseline was not captured");
  await testPage.setViewportSize({ width: 390, height: 844 });

  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileMenuButton.click();
  await testPage
    .getByTestId("mobile-home-menu-card")
    .getByRole("link", { name: "Settings" })
    .click();
  await testPage
    .getByTestId("settings-index")
    .getByRole("link", { name: /^Task Behavior/ })
    .click();

  const card = testPage.getByTestId("session-capacity-settings");
  await expect(card).toBeVisible();
  const toggle = card.getByTestId("session-capacity-enabled");
  const touchTarget = card.getByTestId("session-capacity-enabled-touch-target");
  const touchBox = await touchTarget.boundingBox();
  expect(touchBox).not.toBeNull();
  expect(touchBox!.width).toBeGreaterThanOrEqual(44);
  expect(touchBox!.height).toBeGreaterThanOrEqual(44);

  await toggle.tap();
  const maximum = card.getByTestId("session-capacity-maximum");
  const maximumBox = await maximum.boundingBox();
  expect(maximumBox).not.toBeNull();
  expect(maximumBox!.height).toBeGreaterThanOrEqual(44);
  await maximum.fill("6");

  const saveBar = testPage.getByTestId("settings-floating-save");
  await expect(saveBar).toBeVisible();
  await expectElementsNotToIntersect(touchTarget, saveBar);
  await expectElementsNotToIntersect(maximum, saveBar);

  await saveBar.getByRole("button", { name: "Reset" }).tap();
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  await expect(saveBar).not.toBeVisible();

  await toggle.tap();
  await maximum.fill("6");
  const saveResponse = testPage.waitForResponse(
    (response) =>
      response.request().method() === "PATCH" &&
      new URL(response.url()).pathname === SESSION_CAPACITY_SETTINGS_PATH,
  );
  await saveBar.getByRole("button", { name: "Save changes" }).tap();
  expect((await saveResponse).status()).toBe(200);
  await expect(saveBar).not.toBeVisible();

  await testPage.reload();
  await expect(card.getByTestId("session-capacity-enabled")).toHaveAttribute(
    "aria-checked",
    "true",
  );
  await expect(card.getByTestId("session-capacity-maximum")).toHaveValue("6");
  await expect(card.getByTestId("session-capacity-effective-value")).toHaveText("6");

  await card.getByTestId("session-capacity-enabled").tap();
  await testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" })
    .tap();
  await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();
  await testPage.reload();
  await expect(card.getByTestId("session-capacity-enabled")).toHaveAttribute(
    "aria-checked",
    "false",
  );

  expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
    await testPage.evaluate(() => document.documentElement.clientWidth),
  );
  await expect(testPage.getByTestId("settings-scroll-container")).toHaveCSS("overflow-y", "auto");
});

test("saves the agent-tab close preference with a touch-safe selector", async ({
  testPage,
  apiClient,
}) => {
  const baselineBehavior = (await apiClient.getUserSettings()).settings.agent_tab_close_behavior;
  const savedBehavior = baselineBehavior === "hide_panel" ? "hide_panel" : "delete_session";
  const draftBehavior = savedBehavior === "hide_panel" ? "delete_session" : "hide_panel";
  const behaviorLabel = (behavior: "delete_session" | "hide_panel") =>
    behavior === "hide_panel" ? "Hide panel" : "Delete session";
  let preferencePatchCount = 0;
  const countPreferencePatches = (request: { method: () => string; url: () => string }) => {
    if (
      request.method() === "PATCH" &&
      new URL(request.url()).pathname === "/api/v1/user/settings"
    ) {
      preferencePatchCount += 1;
    }
  };
  testPage.on("request", countPreferencePatches);
  await testPage.setViewportSize({ width: 390, height: 844 });

  try {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileMenuButton.click();
    await testPage
      .getByTestId("mobile-home-menu-card")
      .getByRole("link", { name: "Settings" })
      .click();
    await testPage
      .getByTestId("settings-index")
      .getByRole("link", { name: /^Task Behavior/ })
      .click();

    const card = testPage.getByTestId("agent-tab-close-behavior-card");
    await expect(card).toBeVisible();
    const selector = card.getByRole("combobox");
    const selectorBox = await selector.boundingBox();
    expect(selectorBox).not.toBeNull();
    expect(selectorBox!.height).toBeGreaterThanOrEqual(44);

    await selector.tap();
    await testPage.getByRole("option", { name: behaviorLabel(draftBehavior) }).tap();
    const saveBar = testPage.getByTestId("settings-floating-save");
    await expect(saveBar).toBeVisible();
    await saveBar.getByRole("button", { name: "Reset" }).tap();
    await expect(selector).toHaveText(behaviorLabel(savedBehavior));
    await expect(saveBar).not.toBeVisible();
    expect(preferencePatchCount).toBe(0);

    await selector.tap();
    await testPage.getByRole("option", { name: behaviorLabel(draftBehavior) }).tap();
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .tap();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();

    await testPage.reload();
    await expect(card.getByRole("combobox")).toHaveText(behaviorLabel(draftBehavior));
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
  } finally {
    testPage.off("request", countPreferencePatches);
    await apiClient.saveUserSettings({ agent_tab_close_behavior: baselineBehavior });
  }
});
