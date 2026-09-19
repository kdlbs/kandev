import { expect, test } from "../../fixtures/test-base";
import type {
  SessionCapacitySettingsResponse,
  SessionCapacitySettingsValue,
} from "../../../lib/types/system";
import {
  requestSessionCapacitySettings,
  restoreSessionCapacitySettings,
  SESSION_CAPACITY_SETTINGS_PATH,
} from "../../helpers/session-capacity-settings";

test.describe.serial("Session capacity task behavior settings", () => {
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

  test("finds the setting, stages Reset, and saves live values across reloads", async ({
    testPage,
    apiClient,
  }) => {
    if (!baseline) throw new Error("session capacity settings baseline was not captured");

    await testPage.goto("/settings/preferences/appearance");
    const settingsNav = testPage.getByTestId("app-sidebar-settings-mode");
    await settingsNav.getByRole("searchbox", { name: "Search settings" }).fill("session capacity");
    await settingsNav
      .getByRole("link")
      .filter({ hasText: /^Session capacity/ })
      .click();

    await expect(testPage).toHaveURL(
      /\/settings\/preferences\/task-behavior#setting-session-capacity$/,
    );
    await expect(
      testPage.locator('[data-settings-target="setting-session-capacity"]'),
    ).toHaveAttribute("data-settings-target-highlight", "true");

    const card = testPage.getByTestId("session-capacity-settings");
    const toggle = card.getByTestId("session-capacity-enabled");
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await expect(card.getByTestId("session-capacity-effective-value")).toHaveText(
      "No session limit",
    );
    await expect(card.getByTestId("session-capacity-maximum")).toHaveCount(0);

    await toggle.click();
    const maximum = card.getByTestId("session-capacity-maximum");
    await expect(maximum).toHaveValue(String(baseline.max_sessions));
    await maximum.fill("1");
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Reset" })
      .click();
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await expect(card.getByTestId("session-capacity-maximum")).toHaveCount(0);
    await expect(testPage.getByTestId("settings-floating-save")).toHaveCount(0);

    await toggle.click();
    await maximum.fill("1");
    const firstSave = testPage.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        new URL(response.url()).pathname === SESSION_CAPACITY_SETTINGS_PATH,
    );
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    expect((await firstSave).status()).toBe(200);
    await expect(card.getByTestId("session-capacity-effective-value")).toHaveText("1");
    await expect(card.getByTestId("session-capacity-source")).toHaveText("Saved setting");

    await testPage.reload();
    await expect(card.getByTestId("session-capacity-enabled")).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await expect(card.getByTestId("session-capacity-maximum")).toHaveValue("1");

    await card.getByTestId("session-capacity-maximum").fill("2");
    const secondSave = testPage.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        new URL(response.url()).pathname === SESSION_CAPACITY_SETTINGS_PATH,
    );
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    expect((await secondSave).status()).toBe(200);
    await expect(card.getByTestId("session-capacity-effective-value")).toHaveText("2");

    await card.getByTestId("session-capacity-enabled").click();
    const disableSave = testPage.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        new URL(response.url()).pathname === SESSION_CAPACITY_SETTINGS_PATH,
    );
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    expect((await disableSave).status()).toBe(200);
    await testPage.reload();
    await expect(card.getByTestId("session-capacity-enabled")).toHaveAttribute(
      "aria-checked",
      "false",
    );
    await expect(card.getByTestId("session-capacity-effective-value")).toHaveText(
      "No session limit",
    );

    const persisted = await requestSessionCapacitySettings(apiClient, "GET");
    expect(persisted.settings).toMatchObject({ enabled: false, max_sessions: 2 });
    expect(persisted.effective).toMatchObject({ enabled: false, max_sessions: 0 });
  });

  test("shows an environment override as read-only", async ({ testPage }) => {
    await testPage.route(`**${SESSION_CAPACITY_SETTINGS_PATH}`, async (route) => {
      if (route.request().method() !== "GET") {
        await route.continue();
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          settings: { enabled: false, max_sessions: 5 },
          effective: {
            enabled: true,
            max_sessions: 8,
            source: "environment",
            locked: true,
          },
        } satisfies SessionCapacitySettingsResponse),
      });
    });

    await testPage.goto("/settings/preferences/task-behavior");
    const card = testPage.getByTestId("session-capacity-settings");
    await expect(card.getByTestId("session-capacity-enabled")).toBeDisabled();
    await expect(card.getByTestId("session-capacity-maximum")).toBeDisabled();
    await expect(card.getByTestId("session-capacity-effective-value")).toHaveText("8");
    await expect(card.getByTestId("session-capacity-source")).toHaveText("Environment");
    await expect(card).toContainText("KANDEV_MAX_CONCURRENT_SESSIONS");
    await expect(testPage.getByTestId("settings-floating-save")).toHaveCount(0);
  });
});
