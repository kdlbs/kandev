import type { Page } from "@playwright/test";
import { expect } from "../fixtures/test-base";
import {
  openInstallDialog,
  PACKAGE_PATH,
  PLUGIN_ID,
  uploadPackage,
} from "../tests/plugins/plugin-test-helpers";
import { waitForHttp } from "./causal-waits";

export const TOAST_THEME_CASES = [
  { name: "saved dark", theme: "dark", colorScheme: "light", expected: "dark" },
  { name: "system dark", theme: "system", colorScheme: "dark", expected: "dark" },
  { name: "saved light", theme: "light", colorScheme: "dark", expected: "light" },
] as const;

type ToastThemeCase = (typeof TOAST_THEME_CASES)[number];

export async function verifyPluginToastTheme(page: Page, scenario: ToastThemeCase, mobile = false) {
  await page.emulateMedia({ colorScheme: scenario.colorScheme });
  await page.addInitScript((theme) => localStorage.setItem("theme", theme), scenario.theme);

  if (mobile) {
    await page.goto("/settings/plugins");
    await page.getByTestId("install-plugin-trigger").tap();
    await expect(page.getByTestId("install-plugin-dialog")).toBeVisible();
  } else {
    await openInstallDialog(page);
  }
  await expect(page.locator("html")).toHaveClass(new RegExp(`(^|\\s)${scenario.expected}(\\s|$)`));

  const installed = waitForHttp(page, "POST", /\/api\/plugins\/install$/);
  await uploadPackage(page, PACKAGE_PATH);
  expect((await installed).ok()).toBe(true);

  const notification = page.locator('[data-sonner-toast][data-type="success"]', {
    hasText: "Kandev E2E Fixture Plugin installed",
  });
  await expect(notification).toBeVisible();
  expect(await page.locator("[data-sonner-toaster]").getAttribute("data-sonner-theme")).toBe(
    scenario.expected,
  );

  const colors = await notification.evaluate((element) => {
    const style = getComputedStyle(element);
    const probe = document.createElement("div");
    probe.style.display = "none";
    probe.style.backgroundColor = style.getPropertyValue("--success-bg");
    probe.style.color = style.getPropertyValue("--success-text");
    probe.style.borderColor = style.getPropertyValue("--success-border");
    document.body.append(probe);
    try {
      const palette = getComputedStyle(probe);
      return {
        background: palette.backgroundColor,
        text: palette.color,
        border: palette.borderTopColor,
      };
    } finally {
      probe.remove();
    }
  });
  await expect(notification).toHaveCSS("background-color", colors.background);
  await expect(notification).toHaveCSS("color", colors.text);
  await expect(notification).toHaveCSS("border-top-color", colors.border);
  await expect(page.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible();

  await notification.evaluate(async (element) => {
    await Promise.all(
      element
        .getAnimations()
        .filter((animation) => Number.isFinite(animation.effect?.getComputedTiming().iterations))
        .map((animation) => animation.finished.catch(() => undefined)),
    );
  });
  const box = await notification.boundingBox();
  const viewport = page.viewportSize();
  expect(box).not.toBeNull();
  expect(viewport).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width);
  expect(box!.y + box!.height).toBeLessThanOrEqual(viewport!.height);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
}
