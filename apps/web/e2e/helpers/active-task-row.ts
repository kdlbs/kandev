import { expect, type Locator } from "@playwright/test";

export async function expectActiveTaskRow(row: Locator): Promise<void> {
  await expect(row).toHaveAttribute("data-active", "true");
  await expect(row).toHaveAttribute("aria-current", "true");

  const visualState = await row.evaluate((element) => {
    const style = window.getComputedStyle(element);
    return {
      backgroundColor: style.backgroundColor,
      borderTopStyle: style.borderTopStyle,
      borderTopWidth: style.borderTopWidth,
      borderBottomStyle: style.borderBottomStyle,
      borderBottomWidth: style.borderBottomWidth,
      borderLeftWidth: style.borderLeftWidth,
      borderRightWidth: style.borderRightWidth,
    };
  });

  expect(visualState.backgroundColor).not.toBe("transparent");
  expect(visualState.backgroundColor).not.toBe("rgba(0, 0, 0, 0)");
  expect(visualState.borderTopStyle).toBe("solid");
  expect(visualState.borderTopWidth).toBe("1px");
  expect(visualState.borderBottomStyle).toBe("solid");
  expect(visualState.borderBottomWidth).toBe("1px");
  expect(visualState.borderLeftWidth).toBe("0px");
  expect(visualState.borderRightWidth).toBe("0px");
}

export async function expectActiveTaskRowWithoutColor(row: Locator): Promise<void> {
  await expectActiveTaskRow(row);
  await expect(row.locator("div.absolute.left-0.top-0.bottom-0")).toHaveCount(0);
}
