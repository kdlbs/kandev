import { expect, type Page, type Route } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { waitForHttp } from "../../helpers/causal-waits";

const SETTINGS_URL = "**/api/v1/user/settings";
const SETTINGS_PATH = /\/api\/v1\/user\/settings$/;

// @covers AC-UI-SETTINGS-MANUAL-SAVE-001.1, AC-UI-SETTINGS-MANUAL-SAVE-001.2, AC-UI-SETTINGS-MANUAL-SAVE-001.4
export async function exerciseCustomFontSecondSave(
  page: Page,
  api: ApiClient,
  capture: PrAssetCapture,
  touch: boolean,
) {
  const initial = (await api.getUserSettings()).settings;
  const admitted = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const payloads: Array<{ terminal_font_family?: string }> = [];
  let pendingRoute: Promise<void> | undefined;
  const intercept = async (route: Route) => {
    if (route.request().method() !== "PATCH") return route.continue();
    payloads.push(route.request().postDataJSON());
    if (payloads.length !== 1) return route.continue();
    pendingRoute = (async () => {
      try {
        const response = await route.fetch();
        expect(response.status()).toBe(200);
        admitted.resolve();
        await release.promise;
        await route.fulfill({ response });
      } catch (error) {
        admitted.reject(error);
        throw error;
      }
    })();
    return pendingRoute;
  };

  try {
    await api.saveUserSettings({ terminal_font_family: "OriginalFont" });
    await page.goto("/settings/preferences/terminal-editors");
    const input = page.getByTestId("terminal-font-custom-input");
    await expect(input).toHaveValue("OriginalFont");
    await page.route(SETTINGS_URL, intercept);
    await input.fill("SubmittedFont");
    await input.blur();
    expect((await api.getUserSettings()).settings.terminal_font_family).toBe("OriginalFont");
    const save = page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes", exact: true });
    if (touch) await save.tap();
    else await save.click();
    await admitted.promise;
    expect(payloads).toEqual([expect.objectContaining({ terminal_font_family: "SubmittedFont" })]);
    await expect(
      page
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Saving changes", exact: true }),
    ).toBeDisabled();
    await input.fill("NewerFont");
    await expect(input).toBeFocused();
    const firstAck = waitForHttp(page, "PATCH", SETTINGS_PATH);
    release.resolve();
    await firstAck;
    await expect(input).toBeFocused();
    await expect(input).toHaveValue("NewerFont");
    await expect(input).toHaveAttribute("data-settings-dirty", "true");
    await expect(save).toBeEnabled();
    expect((await api.getUserSettings()).settings.terminal_font_family).toBe("SubmittedFont");
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
    await capture.screenshot("newer-font-dirty", {
      caption:
        "The newer custom font stays dirty and Save remains available after an older save completes.",
    });
    const secondAck = waitForHttp(page, "PATCH", SETTINGS_PATH);
    if (touch) await save.tap();
    else await save.click();
    await secondAck;
    expect(payloads).toHaveLength(2);
    expect(payloads[1].terminal_font_family).toBe("NewerFont");
    await expect(input).toHaveAttribute("data-settings-dirty", "false");
    expect((await api.getUserSettings()).settings.terminal_font_family).toBe("NewerFont");
    await page.reload();
    await expect(page.getByTestId("terminal-font-custom-input")).toHaveValue("NewerFont");
  } finally {
    release.resolve();
    try {
      await pendingRoute;
    } finally {
      await page.unroute(SETTINGS_URL, intercept);
      await api.saveUserSettings({ terminal_font_family: initial.terminal_font_family ?? "" });
    }
  }
}
