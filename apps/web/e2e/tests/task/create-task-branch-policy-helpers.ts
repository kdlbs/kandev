import { expect, type Locator, type Page } from "@playwright/test";

export async function openBranchPolicyDialog(page: Page) {
  const button = page.getByTestId("create-task-button").first();
  const expand = page.getByTestId("sidebar-navigation-expand");
  await expect
    .poll(async () => {
      const receivesPointer = await button.evaluate((element) => {
        const bounds = element.getBoundingClientRect();
        const target = document.elementFromPoint(
          bounds.x + bounds.width / 2,
          bounds.y + bounds.height / 2,
        );
        return target !== null && element.contains(target);
      });
      if (receivesPointer) return true;
      if ((await expand.isVisible()) && (await expand.getAttribute("aria-expanded")) === "false") {
        await expand.click();
      }
      return false;
    })
    .toBe(true);
  await button.click();
}

export async function expectPolicyOptionUsesOneLine(option: Locator, policyName: string) {
  const marker = option.getByText("Policy", { exact: true });
  const name = option.getByText(policyName, { exact: true });
  const [markerBox, nameBox] = await Promise.all([marker.boundingBox(), name.boundingBox()]);

  expect(markerBox).not.toBeNull();
  expect(nameBox).not.toBeNull();
  expect(
    Math.abs(markerBox!.y + markerBox!.height / 2 - (nameBox!.y + nameBox!.height / 2)),
  ).toBeLessThan(3);
  await expect(option.getByText(/Base:/)).toHaveCount(0);
}
