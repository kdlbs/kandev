import { expect, type Locator } from "@playwright/test";

export async function expectActiveTaskRow(row: Locator): Promise<void> {
  await expect(row).toHaveAttribute("data-active", "true");
  await expect(row).toHaveAttribute("aria-current", "true");

  const visualState = await row.evaluate((element) => {
    const style = window.getComputedStyle(element);
    return {
      backgroundColor: style.backgroundColor,
      boxShadow: style.boxShadow,
      ringColor: style.getPropertyValue("--tw-ring-color"),
      primaryColor: style.getPropertyValue("--primary"),
      foregroundColor: style.getPropertyValue("--foreground"),
    };
  });

  expect(visualState.backgroundColor).not.toBe("transparent");
  expect(visualState.backgroundColor).not.toBe("rgba(0, 0, 0, 0)");
  expect(visualState.boxShadow).not.toBe("none");
  expect(visualState.primaryColor).not.toBe("");
  expect(visualState.foregroundColor).not.toBe("");
  expect(visualState.ringColor).toContain(visualState.primaryColor);
  expect(visualState.ringColor).not.toContain(visualState.foregroundColor);
}
