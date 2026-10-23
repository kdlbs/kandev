import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import {
  expectInheritedButtonForeground,
  expectNeutralDisabledButton,
  setButtonTheme,
} from "../../helpers/button-state-colors";

async function expectButtonPalette(button: Locator, palette: "success" | "primary") {
  await expect(button).toHaveCSS("opacity", "1");
  await expect
    .poll(() =>
      button.evaluate((element, palette) => {
        const probe = document.createElement("span");
        probe.style.cssText = `position:absolute;visibility:hidden;background:var(--${palette});color:var(--${palette}-foreground)`;
        element.append(probe);
        try {
          const actual = getComputedStyle(element);
          const expected = getComputedStyle(probe);
          return (
            actual.color === expected.color && actual.backgroundColor === expected.backgroundColor
          );
        } finally {
          probe.remove();
        }
      }, palette),
    )
    .toBe(true);
  await expectInheritedButtonForeground(button);
}

export async function expectSuccessButton(button: Locator) {
  await expectButtonPalette(button, "success");
}

export async function expectPrimaryButton(button: Locator) {
  await expect(button).toBeEnabled();
  await expectButtonPalette(button, "primary");
}

export async function checkNotificationButton(
  page: Page,
  permission: "unsupported" | "granted",
  dark: boolean,
  screenshot: string,
) {
  if (permission === "unsupported") {
    await page.addInitScript(() => {
      Object.defineProperty(window, "Notification", { value: undefined, configurable: true });
    });
  } else {
    await page.context().grantPermissions(["notifications"]);
  }
  await page.goto("/settings/preferences/notifications");
  const button = page.getByTitle("Enable desktop notifications", { exact: true });
  await expect(button).toHaveText(permission === "granted" ? "Enabled" : "Enable");
  await button.scrollIntoViewIfNeeded();
  await setButtonTheme(page, dark);
  await expect(button).toBeDisabled();
  if (permission === "granted") await expectSuccessButton(button);
  else await expectNeutralDisabledButton(button);
  await page.screenshot({ path: screenshot });
}

export async function checkSettingsSaveColors(
  page: Page,
  apiClient: ApiClient,
  dark: boolean,
  screenshot: (state: string) => string,
) {
  const initial = (await apiClient.getUserSettings()).settings.changes_panel_layout;
  const next = initial === "tree" ? "flat" : "tree";
  let release = () => {};
  let held = false;
  let patches = 0;
  await page.route("**/api/v1/user/settings", async (route) => {
    const request = route.request();
    if (request.method() !== "PATCH" || request.postDataJSON()?.changes_panel_layout !== next) {
      await route.continue();
      return;
    }
    patches += 1;
    await new Promise<void>((resolve) => {
      release = resolve;
      held = true;
    });
    const response = await route.fetch();
    await route.fulfill({ response });
  });
  try {
    await page.goto("/settings/preferences/appearance");
    const layout = page.getByTestId("changes-panel-layout-select");
    await layout.scrollIntoViewIfNeeded();
    await layout.click();
    await page.getByRole("option", { name: next === "tree" ? "Tree" : "Flat list" }).click();
    const surface = page.getByTestId("settings-floating-save");
    const save = surface.getByRole("button", { name: "Save changes", exact: true });
    await expect(save).toBeEnabled();
    await setButtonTheme(page, dark);
    await expectSuccessButton(save);
    await page.screenshot({ path: screenshot("enabled") });
    await save.click();
    await expect.poll(() => held).toBe(true);
    await expect(surface).toHaveAttribute("data-status", "saving");
    const pending = surface.getByRole("button", { name: "Saving changes", exact: true });
    await expectNeutralDisabledButton(pending);
    await pending.evaluate((element: HTMLButtonElement) => element.click());
    expect(patches).toBe(1);
    await page.screenshot({ path: screenshot("pending") });
    release();
    await expect(surface).toHaveAttribute("data-status", "saved");
    const saved = surface.getByRole("button", { name: "Saved", exact: true });
    await expect(saved).toBeDisabled();
    await expectSuccessButton(saved);
    await page.screenshot({ path: screenshot("saved") });
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.changes_panel_layout)
      .toBe(next);
    expect(patches).toBe(1);
  } finally {
    release();
    await page.unrouteAll({ behavior: "wait" });
    await apiClient.saveUserSettings({ changes_panel_layout: initial ?? "flat" });
  }
}

export async function expectNeutralCanvasSetup(button: Locator) {
  await expect(button).toBeVisible();
  await expect(button).toBeEnabled();
  await expect(button).toHaveCSS("opacity", "1");
  await expect(button).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect
    .poll(() =>
      button.evaluate((element) => {
        const probe = document.createElement("span");
        probe.style.cssText = "position:absolute;visibility:hidden;color:var(--muted-foreground)";
        element.append(probe);
        try {
          return getComputedStyle(element).color === getComputedStyle(probe).color;
        } finally {
          probe.remove();
        }
      }),
    )
    .toBe(true);
}
