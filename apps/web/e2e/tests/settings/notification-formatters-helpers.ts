import type { Page, TestInfo } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  installFixturePlugin,
  uninstallFixturePlugin,
  PLUGIN_ID,
} from "../../helpers/plugin-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

export async function notificationFormatterScenario(
  page: Page,
  api: ApiClient,
  info: TestInfo,
  mobile: boolean,
) {
  let providerId = "";
  try {
    await installFixturePlugin(page);
    const created = await api.rawRequest("POST", "/api/v1/notification-providers", {
      name: "Formatter fixture",
      type: "apprise",
      config: { urls: "json://example.invalid" },
      enabled: false,
      events: [],
    });
    expect(created.ok).toBe(true);
    providerId = (await created.json()).id;
    await page.goto("/settings/preferences/notifications");
    const row = page.getByTestId(`notification-formatter-${providerId}`);
    const select = row.getByRole("combobox");
    await expect(select).toBeEnabled();
    const options = await select.locator("option").allTextContents();
    expect(options.some((label) => label.includes(PLUGIN_ID))).toBe(true);
    await select.selectOption({ label: `${PLUGIN_ID} / fixture-text (1.0.0)` });
    const sound = page.getByRole("switch", { name: "Enable notification sound" });
    const previousSound = await sound.isChecked();
    await sound.click();
    const preparation = waitForHttp(page, "POST", /\/actions\/notification\.prepare$/);
    const preparedReadback = waitForHttp(page, "GET", /\/formatter$/);
    await page.getByRole("button", { name: "Save changes", exact: true }).click();
    expect((await preparation).ok()).toBe(true);
    const unbound = await (await preparedReadback).json();
    expect(unbound.binding?.plugin_id ?? "").toBe("");
    await expect(sound).toHaveAttribute("data-settings-dirty", "false");
    expect(
      await page.evaluate(() =>
        JSON.parse(localStorage.getItem("kandev.notifications.sound") ?? "null"),
      ),
    ).toMatchObject({ enabled: !previousSound });
    await expect(page.getByTestId("settings-floating-save")).toHaveAttribute(
      "data-status",
      "dirty",
    );
    await expect(row.getByTestId("fixture-formatter-status")).toHaveText(
      "Configuration prepared. Activate it explicitly.",
    );
    const preview = waitForHttp(page, "POST", /\/formatter\/preview$/);
    await row.getByTestId("fixture-formatter-preview").click();
    expect((await preview).ok()).toBe(true);
    await expect(row.getByTestId("fixture-formatter-status")).toHaveText(
      "Fixture Agent session failed",
    );
    const activation = waitForHttp(page, "PUT", /\/formatter$/);
    await row.getByTestId("fixture-formatter-activate").click();
    expect((await activation).ok()).toBe(true);
    await expect(row.getByTestId("fixture-formatter-status")).toHaveText(
      "Fixture formatter active",
    );
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    if (mobile) {
      const box = await row.getByTestId("fixture-formatter-activate").boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(44);
    }
    await page.screenshot({
      path: info.outputPath(mobile ? "formatter-phone.png" : "formatter-desktop.png"),
      fullPage: true,
    });
    await select.selectOption("");
    const cleared = waitForHttp(page, "PUT", /\/formatter$/);
    await page.getByRole("button", { name: "Save changes", exact: true }).click();
    expect((await cleared).ok()).toBe(true);
    const settings = await api.rawRequest(
      "GET",
      `/api/v1/notification-providers/${providerId}/formatter`,
    );
    expect((await settings.json()).binding.plugin_id).toBe("");
  } finally {
    if (providerId) await api.rawRequest("DELETE", `/api/v1/notification-providers/${providerId}`);
    await uninstallFixturePlugin(api);
  }
}
