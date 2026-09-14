import { expect, type Page } from "@playwright/test";

export type DisplaySettingsGroup = "filters" | "sort" | "preview" | "list-rows";

export async function expandDisplaySettingsGroup(
  page: Page,
  group: DisplaySettingsGroup,
  surface: "desktop" | "mobile" = "desktop",
): Promise<void> {
  const prefix = surface === "mobile" ? "mobile-display-settings" : "display-settings";
  const toggle = page.getByTestId(`${prefix}-${group}-toggle`);
  await expect(toggle).toBeVisible();
  if ((await toggle.getAttribute("aria-expanded")) !== "true") {
    if (surface === "mobile") {
      await toggle.tap();
    } else {
      await toggle.click();
    }
  }
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
}
