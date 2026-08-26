import { expect, type Locator } from "@playwright/test";

export async function expectActiveTaskRow(row: Locator): Promise<void> {
  await expect(row).toHaveAttribute("data-active", "true");
  await expect(row).toHaveAttribute("aria-current", "true");

  const visualState = await row.evaluate((element) => {
    const style = window.getComputedStyle(element);
    return {
      backgroundColor: style.backgroundColor,
      boxShadow: style.boxShadow,
    };
  });

  expect(visualState.backgroundColor).not.toBe("transparent");
  expect(visualState.backgroundColor).not.toBe("rgba(0, 0, 0, 0)");
  expect(visualState.boxShadow).toBe("none");
}
