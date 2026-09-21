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
  await testPage.getByTestId("app-nav-sheet").getByRole("link", { name: "Settings" }).click();
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
